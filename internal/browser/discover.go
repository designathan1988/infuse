package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Finding the installed Chromium-family browsers.

// chromiumCandidates lists the installed browsers, the preferred first.
func chromiumCandidates() []string {
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
	var out []string
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func findChromium() string {
	if c := chromiumCandidates(); len(c) > 0 {
		return c[0]
	}
	return ""
}
