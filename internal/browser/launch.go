package browser

// Launching the windowless Chromium: finding the executable, waiting for its debugging port and
// connecting to its first page. A profile left corrupt by an abrupt exit is replaced by a fresh one.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func findChromium() string {
	var cands []string
	switch runtime.GOOS {
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				cands = append(cands, filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`), filepath.Join(base, `Google\Chrome\Application\chrome.exe`))
			}
		}
	case "darwin":
		cands = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "/Applications/Chromium.app/Contents/MacOS/Chromium"}
	default:
		for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"} {
			if p, err := exec.LookPath(n); err == nil {
				cands = append(cands, p)
			}
		}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// startCDP starts the browser on profile; a profile left corrupt by an abrupt exit (a Crashpad
// database Chrome refuses) is replaced by a fresh one instead of leaving the tool unusable.
func startCDP(profile string) (*Browser, error) {
	b, err := launchCDP(profile)
	if err == nil {
		return b, nil
	}
	fresh := profile + "-" + time.Now().Format("20060102-150405")
	b, freshErr := launchCDP(fresh)
	if freshErr != nil {
		return nil, fmt.Errorf("%v (a fresh profile also failed: %v)", err, freshErr)
	}
	return b, nil
}

func launchCDP(profile string) (*Browser, error) {
	exe := findChromium()
	if exe == "" {
		return nil, errors.New("nenhum navegador Edge, Chrome ou Chromium encontrado neste computador")
	}
	_ = os.MkdirAll(profile, 0o755)
	_ = os.Remove(filepath.Join(profile, "DevToolsActivePort"))
	cmd := exec.Command(exe, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-blink-features=AutomationControlled", "--window-size=1280,800", "about:blank")
	process.NoConsoleWindow(cmd)
	var launch bytes.Buffer
	cmd.Stdout, cmd.Stderr = &launch, &launch
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	port := waitForPort(profile)
	if port == "" {
		_ = cmd.Process.Kill()
		detail := strings.TrimSpace(launch.String())
		if detail != "" {
			return nil, fmt.Errorf("the browser did not open the debugging port: %s", core.Truncate(detail, 400))
		}
		return nil, errors.New("the browser did not open the debugging port")
	}
	wsURL, targetID, ok := pageTarget(port)
	if !ok {
		_ = cmd.Process.Kill()
		return nil, errors.New("the browser has no page")
	}
	ws, err := wsDial(wsURL)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	b := &Browser{port: port, profile: profile, cmd: cmd, targetID: targetID, ws: ws, pending: map[int64]chan json.RawMessage{}, errs: map[int64]string{}, lastUse: time.Now()}
	go b.readLoop()
	b.prepare()
	return b, nil
}

func waitForPort(profile string) string {
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			if port := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0]); port != "" {
				return port
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}

// pageTarget returns the websocket address and target id of the first page of a running browser.
func pageTarget(port string) (string, string, bool) {
	var targets []struct {
		Type, ID, WebSocketDebuggerURL string
	}
	for i := 0; i < 30; i++ {
		if resp, err := http.Get("http://127.0.0.1:" + port + "/json/list"); err == nil {
			_ = json.NewDecoder(resp.Body).Decode(&targets)
			resp.Body.Close()
		}
		if len(targets) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, t := range targets {
		if t.Type == "page" {
			return t.WebSocketDebuggerURL, t.ID, true
		}
	}
	return "", "", false
}
