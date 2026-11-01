package base

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStarterAndCustomBases(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, "web", nil); err != nil {
		t.Fatal(err)
	}
	b := Load(root)
	if b == nil || b.Name != "web" || !b.Declares("src/components/a.js") {
		t.Fatalf("%+v", b)
	}
	if b.Roles["src/components"] != "UI components" || !strings.Contains(b.Summary(), "responsibilities:") {
		t.Fatalf("module roles: %+v %q", b.Roles, b.Summary())
	}
	if CheckWrite(root, "src/components/a.js", true) != "" {
		t.Fatal("a declared folder was refused")
	}
	if CheckWrite(root, "scripts/deploy.js", true) == "" {
		t.Fatal("code outside the base was accepted")
	}
	if CheckWrite(root, "README.md", false) != "" || CheckWrite(root, "package.json", false) != "" {
		t.Fatal("root files must be allowed")
	}
	custom := &Base{Name: "mine", Folders: []string{"app", "app/core"}}
	if err := Save(root, "", custom); err != nil {
		t.Fatal(err)
	}
	if b := Load(root); b == nil || b.Name != "mine" || !b.Declares("app/core/x.go") {
		t.Fatalf("%+v", b)
	}
	if err := Save(root, "unknown-base", nil); err == nil {
		t.Fatal("an unknown starter was accepted")
	}
}

func TestNoBaseMeansNoCheck(t *testing.T) {
	root := t.TempDir()
	if b := Load(root); b != nil {
		t.Fatalf("%+v", b)
	}
	if CheckWrite(root, "anywhere/x.go", true) != "" {
		t.Fatal("a project without a base was checked")
	}
	_ = os.Remove(filepath.Join(root, ".infuse", "base.json"))
}
