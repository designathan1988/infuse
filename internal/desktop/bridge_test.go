package desktop

import (
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"testing"
)

// stubDialogs stands in for the OS dialog: the test decides what the user "picked".
type stubDialogs struct {
	folder string
	files  []string
	calls  []string
}

func (s *stubDialogs) PickFolder(title string) (string, error) {
	s.calls = append(s.calls, "folder:"+title)
	return s.folder, nil
}
func (s *stubDialogs) PickFiles(title string) ([]string, error) {
	s.calls = append(s.calls, "files:"+title)
	return s.files, nil
}

type stubHost struct {
	focused bool
	notes   []string
}

func (h *stubHost) Focused() bool             { return h.focused }
func (h *stubHost) Notify(title, body string) { h.notes = append(h.notes, title+"|"+body) }

func args(v ...any) []json.RawMessage {
	var out []json.RawMessage
	for _, x := range v {
		b, _ := json.Marshal(x)
		out = append(out, b)
	}
	return out
}

func TestBridgePickFolderRemembersProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("INFUSE_HOME", home)
	dir := t.TempDir()
	d := &stubDialogs{folder: dir}
	b := &Bridge{Dialogs: d, Host: &stubHost{}}
	res, err := b.Call("pickFolder", nil)
	if err != nil || res != filepath.Clean(dir) {
		t.Fatalf("got %v, %v", res, err)
	}
	if len(d.calls) != 1 || d.calls[0][:7] != "folder:" {
		t.Fatalf("dialog not used: %v", d.calls)
	}
	list := loadProjects()
	if len(list) != 1 || list[0].Root != filepath.Clean(dir) {
		t.Fatalf("project not remembered: %+v", list)
	}
}

func TestBridgePickFolderCancelled(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	b := &Bridge{Dialogs: &stubDialogs{}, Host: &stubHost{}}
	res, err := b.Call("pickFolder", nil)
	if err != nil || res != nil {
		t.Fatalf("cancel must give nil: %v %v", res, err)
	}
	if len(loadProjects()) != 0 {
		t.Fatal("cancel must not record a project")
	}
}

func TestBridgePickFilesAndDrop(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	img := filepath.Join(dir, "b.PNG")
	_ = os.WriteFile(txt, []byte("hello"), 0o644)
	_ = os.WriteFile(img, []byte("png"), 0o644)
	b := &Bridge{Dialogs: &stubDialogs{files: []string{txt, img, filepath.Join(dir, "missing")}}, Host: &stubHost{}}
	res, err := b.Call("pickFiles", nil)
	if err != nil {
		t.Fatal(err)
	}
	list := res.([]Attachment)
	if len(list) != 2 || list[0].Size != 5 || list[0].Image || !list[1].Image {
		t.Fatalf("%+v", list)
	}
	dropped, _ := b.Call("drop", args([]string{dir, txt}))
	dr := dropped.(DropResult)
	if len(dr.Projects) != 1 || len(dr.Attachments) != 1 {
		t.Fatalf("%+v", dr)
	}
	if loadProjects()[0].Root != filepath.Clean(dir) {
		t.Fatal("dropped folder must become a project")
	}
}

func TestBridgeNotifyOnlyWhenUnfocused(t *testing.T) {
	h := &stubHost{focused: true}
	b := &Bridge{Dialogs: &stubDialogs{}, Host: h}
	_, _ = b.Call("notify", args("t", "b"))
	if len(h.notes) != 0 {
		t.Fatal("focused window must not notify")
	}
	h.focused = false
	_, _ = b.Call("notify", args("t", "b"))
	if len(h.notes) != 1 || h.notes[0] != "t|b" {
		t.Fatalf("%v", h.notes)
	}
}

func TestBridgeUnknownMethod(t *testing.T) {
	b := &Bridge{Dialogs: &stubDialogs{}, Host: &stubHost{}}
	if _, err := b.Call("nope", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestRememberProjectKeepsOrder(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	RememberProject("/a")
	RememberProject("/b")
	RememberProject("/a") // seen again: stays where it is
	l := loadProjects()
	if len(l) != 2 || l[0].Root != "/b" || l[1].Root != "/a" {
		t.Fatalf("%+v", l)
	}
}

func TestIconPNG(t *testing.T) {
	b := iconPNG(16)
	if len(b) < 8 || string(b[1:4]) != "PNG" {
		t.Fatal("not a png")
	}
}

func loadProjects() []config.ProjectEntry { return config.LoadProjects() }
func RememberProject(root string)         { config.RememberProject(root) }
func iconPNG(n int) []byte                { return IconPNG(n) }

type Attachment = core.Attachment
