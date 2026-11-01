//go:build windows && !cgo

package mountwin

import (
	"infuse/internal/base"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

func mountFixture(t *testing.T) *FileSystem {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"src/main.go":        "package main\n",
		".infuse/oracle.txt": "criteria\n",
		".env":               "SECRET=1\n",
		"README.md":          "readme\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := New(root, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fs.Destroy)
	return fs
}

func TestMountReadsAndWrites(t *testing.T) {
	fs := mountFixture(t)
	var st fuse.Stat_t
	if code := fs.Getattr("/src/main.go", &st, 0); code != 0 || st.Size == 0 || st.Mode&fuse.S_IFREG == 0 {
		t.Fatalf("Getattr: code=%d stat=%+v", code, st)
	}
	if code, fh := fs.Open("/src/main.go", fuse.O_RDONLY); code != 0 {
		t.Fatalf("Open: %d", code)
	} else {
		buf := make([]byte, 16)
		if n := fs.Read("/src/main.go", buf, 0, fh); n <= 0 {
			t.Fatalf("Read returned %d", n)
		}
		fs.Release("/src/main.go", fh)
	}
	code, fh := fs.Create("/src/new.go", fuse.O_CREAT|fuse.O_WRONLY, 0o644)
	if code != 0 {
		t.Fatalf("Create: %d", code)
	}
	if n := fs.Write("/src/new.go", []byte("package new\n"), 0, fh); n <= 0 {
		t.Fatalf("Write returned %d", n)
	}
	fs.Release("/src/new.go", fh)
	if b, err := os.ReadFile(filepath.Join(fs.root, "src", "new.go")); err != nil || string(b) != "package new\n" {
		t.Fatalf("written file: %q err=%v", b, err)
	}
	if code := fs.Rename("/src/new.go", "/src/renamed.go"); code != 0 {
		t.Fatalf("Rename: %d", code)
	}
	if code := fs.Unlink("/src/renamed.go"); code != 0 {
		t.Fatalf("Unlink: %d", code)
	}
}

func TestMountRefusesProtectedAndOutsideBase(t *testing.T) {
	fs := mountFixture(t)
	for _, path := range []string{"/.infuse/oracle.txt", "/.env"} {
		if code, _ := fs.Create(path, fuse.O_CREAT|fuse.O_WRONLY, 0o644); code != -fuse.EPERM {
			t.Fatalf("Create(%s) = %d, want EPERM", path, code)
		}
		if code := fs.Unlink(path); code != -fuse.EPERM {
			t.Fatalf("Unlink(%s) = %d, want EPERM", path, code)
		}
	}
	basePath := filepath.Join(fs.root, ".infuse", "base.json")
	if err := os.WriteFile(basePath, []byte(`{"base":"web"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fs.base = base.Load(fs.root)
	if code, _ := fs.Create("/server/app.go", fuse.O_CREAT|fuse.O_WRONLY, 0o644); code != -fuse.EPERM {
		t.Fatalf("out-of-base code was writable: %d", code)
	}
	if code, fh := fs.Create("/src/app.go", fuse.O_CREAT|fuse.O_WRONLY, 0o644); code != 0 {
		t.Fatalf("in-base code was refused: %d", code)
	} else {
		fs.Release("/src/app.go", fh)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(fs.root, "link")); err != nil {
		return // creating symlinks needs a privilege this machine may not grant
	}
	if code, _ := fs.Create("/link/escape.txt", fuse.O_CREAT|fuse.O_WRONLY, 0o644); code != -fuse.EPERM && code != -fuse.EACCES {
		t.Fatalf("a write through an outside symlink = %d, want a denial", code)
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); err == nil {
		t.Fatal("the symlink escape wrote outside the root")
	}
}

// The real WinFsp mount serves the project on a drive and enforces the same write policy.
func TestMountServesAProject(t *testing.T) {
	if st, err := os.Stat(`C:\Program Files (x86)\WinFsp\bin\winfsp-x64.dll`); err != nil || st.IsDir() {
		t.Skip("WinFsp is not installed")
	}
	fs := mountFixture(t)
	drive := freeDrive(t)
	if drive == "" {
		t.Skip("no free drive letter")
	}
	host := fuse.NewFileSystemHost(fs)
	host.SetCapCaseInsensitive(true)
	mounted := make(chan bool, 1)
	go func() { mounted <- host.Mount(drive+":", nil) }()
	defer host.Unmount()
	path := drive + `:\src\main.go`
	waitForMount(t, path)
	if b, err := os.ReadFile(path); err != nil || len(b) == 0 {
		t.Fatalf("read through the mount: %q err=%v", b, err)
	}
	if err := os.WriteFile(drive+`:\src\mounted.txt`, []byte("mounted"), 0o644); err != nil {
		t.Fatalf("write through the mount: %v", err)
	}
	if err := os.WriteFile(drive+`:\.infuse\oracle.txt`, []byte("tampered"), 0o644); err == nil {
		t.Fatal("the mount wrote a protected record")
	}
	host.Unmount()
	select {
	case ok := <-mounted:
		if !ok {
			t.Fatal("WinFsp mount failed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the mount did not stop")
	}
}

func freeDrive(t *testing.T) string {
	t.Helper()
	for c := 'R'; c <= 'Z'; c++ {
		drive := string(c)
		if _, err := os.Stat(drive + `:\`); err != nil {
			return drive
		}
	}
	return ""
}

func waitForMount(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the mount did not appear")
}

func TestMountReaddir(t *testing.T) {
	fs := mountFixture(t)
	seen := map[string]bool{}
	code := fs.Readdir("/src", func(name string, st *fuse.Stat_t, off int64) bool {
		seen[name] = true
		return true
	}, 0, 0)
	if code != 0 || !seen["main.go"] || !seen["."] {
		t.Fatalf("Readdir: code=%d names=%v", code, seen)
	}
}
