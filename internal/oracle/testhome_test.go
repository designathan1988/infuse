package oracle

import (
	"infuse/internal/config"
	"infuse/internal/platform/gitx"
	"infuse/internal/sandbox"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// serveEnv makes this test binary the snapshot's static file server.
const serveEnv = "ORACLETEST_SERVE"

// TestMain plays the __sandbox role, as the real binary does, so the executor's containment runs end
// to end, and the file server the server checks start from the snapshot.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == sandbox.Role {
		os.Exit(sandbox.Main(os.Args[2:]))
	}
	if os.Getenv(serveEnv) == "1" {
		serveCurrentFolder()
		return
	}
	os.Exit(m.Run())
}

func serveCurrentFolder() {
	ln, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv("PORT"))
	if err != nil {
		os.Exit(3)
	}
	_ = http.Serve(ln, http.FileServer(http.Dir(".")))
}

func runGit(root string, args ...string) (string, error) { return gitx.Run(root, args...) }

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	old := config.SecretBackend
	config.SecretBackend = config.MemSecrets{}
	t.Cleanup(func() { config.SecretBackend = old })
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		if out, err := gitx.Run(root, args...); err != nil {
			t.Fatal(out)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o644)
	_, _ = gitx.Run(root, "add", "-A")
	if out, err := gitx.Run(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(out)
	}
	return root
}

func gitOutput(root string, args ...string) string { return gitx.Output(root, args...) }
