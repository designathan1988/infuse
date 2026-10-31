package workspace

// Dev servers for the Browser pane, declared in <project>/.infuse/launch.json or .claude/launch.json:
// {"configurations":[{"name","runtimeExecutable","runtimeArgs","port","cwd","env","url"}]}.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type LaunchConfig struct {
	Name              string            `json:"name"`
	RuntimeExecutable string            `json:"runtimeExecutable"`
	RuntimeArgs       []string          `json:"runtimeArgs"`
	Port              int               `json:"port"`
	Cwd               string            `json:"cwd,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	URL               string            `json:"url,omitempty"`
}

type devServer struct {
	cmd     *exec.Cmd
	out     *bytes.Buffer
	started time.Time
	exited  bool
}

var (
	devServers   = map[string]*devServer{}
	devServersMu sync.Mutex
)

func LaunchConfigs(root string) []LaunchConfig {
	var f struct {
		Configurations []LaunchConfig `json:"configurations"`
	}
	for _, p := range []string{filepath.Join(root, core.InfuseDirName, "launch.json"), filepath.Join(root, ".claude", "launch.json")} {
		if core.ReadJSON(p, &f) == nil && len(f.Configurations) > 0 {
			break
		}
	}
	for i := range f.Configurations {
		if f.Configurations[i].Port == 0 {
			f.Configurations[i].Port = 3000
		}
		if f.Configurations[i].URL == "" {
			f.Configurations[i].URL = "http://localhost:" + strconv.Itoa(f.Configurations[i].Port)
		}
	}
	return f.Configurations
}

func portOpen(port int) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func DevServers(root string) []map[string]any {
	out := []map[string]any{}
	devServersMu.Lock()
	defer devServersMu.Unlock()
	for _, c := range LaunchConfigs(root) {
		d := devServers[root+"|"+c.Name]
		running := d != nil && !d.exited
		log := ""
		if d != nil {
			b := d.out.Bytes()
			if len(b) > 4000 {
				b = b[len(b)-4000:]
			}
			log = string(b)
		}
		out = append(out, map[string]any{"name": c.Name, "port": c.Port, "url": c.URL, "running": running, "listening": portOpen(c.Port), "log": log,
			"command": launchCommandLine(c), "approved": launchApproved(root, c)})
	}
	return out
}

// ErrLaunchNeedsApproval: the command was never approved by the user (or changed since).
type ErrLaunchNeedsApproval struct{ Command string }

func (e ErrLaunchNeedsApproval) Error() string {
	return "este comando de .infuse/launch.json precisa da sua aprovação antes de rodar: " + e.Command
}

func StartDevServer(root, name string) error {
	for _, c := range LaunchConfigs(root) {
		if c.Name != name {
			continue
		}
		if !launchApproved(root, c) {
			return ErrLaunchNeedsApproval{launchCommandLine(c)}
		}
		devServersMu.Lock()
		defer devServersMu.Unlock()
		if d := devServers[root+"|"+name]; d != nil && !d.exited {
			return nil
		}
		cmd := exec.Command(c.RuntimeExecutable, c.RuntimeArgs...)
		cmd.Dir = root
		if c.Cwd != "" {
			cmd.Dir = filepath.Join(root, c.Cwd)
		}
		cmd.Env = os.Environ()
		for k, v := range c.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		cmd.Env = append(cmd.Env, "PORT="+strconv.Itoa(c.Port))
		buf := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = buf, buf
		process.NoConsoleWindow(cmd)
		if err := cmd.Start(); err != nil {
			return err
		}
		d := &devServer{cmd: cmd, out: buf, started: time.Now()}
		devServers[root+"|"+name] = d
		go func() { _ = cmd.Wait(); devServersMu.Lock(); d.exited = true; devServersMu.Unlock() }()
		return nil
	}
	return errors.New("configuração não encontrada em .infuse/launch.json ou .claude/launch.json")
}

func StopDevServer(root, name string) {
	devServersMu.Lock()
	d := devServers[root+"|"+name]
	devServersMu.Unlock()
	if d == nil || d.cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(d.cmd.Process.Pid))
		process.NoConsoleWindow(kill)
		_ = kill.Run()
	} else {
		_ = d.cmd.Process.Kill()
	}
}

// ---------------------------------------------------------------- launch approval

// A dev-server command lives in the project, where the agent can write. It runs only after the user
// approves that exact command (executable, arguments, folder, environment) — approvals live in
// ~/.infuse, out of the agent's reach.
var launchMu sync.Mutex

func launchFingerprint(root string, c LaunchConfig) string {
	b, _ := json.Marshal([]any{filepath.Clean(root), c.Name, c.RuntimeExecutable, c.RuntimeArgs, c.Cwd, c.Env})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func approvalsFile() string { return filepath.Join(core.InfuseHome(), "approved-launch.json") }

func launchApproved(root string, c LaunchConfig) bool {
	launchMu.Lock()
	defer launchMu.Unlock()
	var m map[string]bool
	_ = core.ReadJSON(approvalsFile(), &m)
	return m[launchFingerprint(root, c)]
}

func approveLaunch(root string, c LaunchConfig) error {
	launchMu.Lock()
	defer launchMu.Unlock()
	m := map[string]bool{}
	_ = core.ReadJSON(approvalsFile(), &m)
	m[launchFingerprint(root, c)] = true
	return core.WriteJSON(approvalsFile(), m)
}

// ApproveLaunch records the user's approval of a launch command.
func ApproveLaunch(root string, c LaunchConfig) error { return approveLaunch(root, c) }

func launchCommandLine(c LaunchConfig) string {
	parts := append([]string{c.RuntimeExecutable}, c.RuntimeArgs...)
	s := strings.Join(parts, " ")
	if c.Cwd != "" {
		s += "   (folder: " + c.Cwd + ")"
	}
	for k, v := range c.Env {
		s += "   " + k + "=" + v
	}
	return s
}

// LaunchApproved reports whether the user approved this exact command.
func LaunchApproved(root string, c LaunchConfig) bool { return launchApproved(root, c) }

// LaunchCommandLine is the command as shown to the user when asking for approval.
func LaunchCommandLine(c LaunchConfig) string { return launchCommandLine(c) }
