package main

// Dev servers for the Browser pane, declared in <project>/.infuse/launch.json or .claude/launch.json:
// {"configurations":[{"name","runtimeExecutable","runtimeArgs","port","cwd","env","url"}]}.

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

type launchConfig struct {
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

func launchConfigs(root string) []launchConfig {
	var f struct {
		Configurations []launchConfig `json:"configurations"`
	}
	for _, p := range []string{filepath.Join(root, infuseDirName, "launch.json"), filepath.Join(root, ".claude", "launch.json")} {
		if readJSON(p, &f) == nil && len(f.Configurations) > 0 {
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
	for _, c := range launchConfigs(root) {
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
		out = append(out, map[string]any{"name": c.Name, "port": c.Port, "url": c.URL, "running": running, "listening": portOpen(c.Port), "log": log})
	}
	return out
}

func StartDevServer(root, name string) error {
	for _, c := range launchConfigs(root) {
		if c.Name != name {
			continue
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
		noConsoleWindow(cmd)
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
		noConsoleWindow(kill)
		_ = kill.Run()
	} else {
		_ = d.cmd.Process.Kill()
	}
}
