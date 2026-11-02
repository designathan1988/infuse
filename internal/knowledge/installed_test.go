package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The version in use wins over the manifest's range, in every ecosystem.
func TestInstalledVersionsOverManifestRanges(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"package.json":                          `{"dependencies":{"react":"^18.0.0","left-pad":"^1.0.0"},"devDependencies":{"@scope/tool":"*"}}`,
		"node_modules/react/package.json":       `{"version":"18.3.1"}`,
		"node_modules/@scope/tool/package.json": `{"version":"2.0.1"}`,
		"package-lock.json":                     `{"packages":{"node_modules/left-pad":{"version":"1.3.0"},"node_modules/react":{"version":"18.0.0"}}}`,
		"go.mod":                                "module x\n\ngo 1.22\n\nrequire (\n\tgithub.com/fsnotify/fsnotify v1.7.0\n\tgolang.org/x/sys v0.20.0 // indirect\n\tgithub.com/gone/away v1.0.0\n)\n",
		"go.sum":                                "github.com/fsnotify/fsnotify v1.7.0 h1:abc=\ngithub.com/fsnotify/fsnotify v1.7.0/go.mod h1:def=\n",
		"Cargo.lock":                            "version = 3\n\n[[package]]\nname = \"mine\"\nversion = \"0.1.0\"\n\n[[package]]\nname = \"serde\"\nversion = \"1.0.203\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\n",
		"poetry.lock":                           "[[package]]\nname = \"Django\"\nversion = \"5.1.1\"\ndescription = \"x\"\n",
	})
	want := map[string]string{
		"react": "https://www.npmjs.com/package/react/v/18.3.1", "left-pad": "https://www.npmjs.com/package/left-pad/v/1.3.0",
		"@scope/tool":                  "https://www.npmjs.com/package/@scope/tool/v/2.0.1",
		"github.com/fsnotify/fsnotify": "https://pkg.go.dev/github.com/fsnotify/fsnotify@v1.7.0",
		"serde":                        "https://docs.rs/serde/1.0.203", "django": "https://pypi.org/project/django/5.1.1/",
	}
	got := map[string]string{}
	for _, d := range InstalledDependencies(root) {
		got[d.Name] = d.DocURL()
	}
	for name, url := range want {
		if got[name] != url {
			t.Errorf("%s: %q, want %q", name, got[name], url)
		}
	}
	for _, absent := range []string{"mine", "golang.org/x/sys", "github.com/gone/away"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s is not a dependency in use", absent)
		}
	}
	if InstalledVersion(root, "React") != "18.3.1" {
		t.Fatal("lookup by name")
	}
}

// Exact pins in requirements.txt are installed versions; a range says nothing about what is installed.
func TestRequirementsPins(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"requirements.txt": "flask==3.0.3\nrequests>=2\n# comment\npytest[extra]==8.3.3 ; python_version > '3.8'\n"})
	got := map[string]string{}
	for _, d := range InstalledDependencies(root) {
		got[d.Name] = d.Version
	}
	if got["flask"] != "3.0.3" || got["pytest"] != "8.3.3" {
		t.Fatalf("%v", got)
	}
	if _, ok := got["requests"]; ok {
		t.Fatal("a range was taken for an installed version")
	}
}

// What infuse generated in an instruction file is not the user's instructions: paths in it do not
// become required reading, and a file holding only that block is not an instruction file.
func TestGeneratedBlocksDoNotBecomeLocks(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"CLAUDE.md":      "<!-- infuse:begin grounds (generated) -->\nsee docs/secret.md\n<!-- infuse:end grounds -->\n",
		"AGENTS.md":      "Follow docs/real.md to the letter\n\n<!-- infuse:begin grounds (generated) -->\nsee docs/secret.md\n<!-- infuse:end grounds -->\n",
		"docs/secret.md": "x\n",
		"docs/real.md":   "y\n",
	})
	InvalidateRules(root)
	r := LoadProjectRules(root)
	if strings.Contains(r.Text, "secret") {
		t.Fatalf("%q", r.Text)
	}
	if strings.Join(r.Files, ",") != "AGENTS.md" {
		t.Fatalf("%v", r.Files)
	}
	if strings.Join(r.Required, ",") != "docs/real.md" {
		t.Fatalf("%v", r.Required)
	}
}

// The grounds carry what infuse knows, and are the same text for the same project.
func TestGroundsAreDeterministic(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"package.json":                    `{"dependencies":{"react":"^18.0.0"}}`,
		"node_modules/react/package.json": `{"version":"18.3.1"}`,
		".infuse/oracle/checks.json":      `{"checks":[{"id":"t","kind":"file","path":"a","description":"a exists","expect":{"exists":true}}]}`,
	})
	one, two := Grounds(root), Grounds(root)
	if one != two || !strings.Contains(one, "a exists") || !strings.Contains(one, "react 18.3.1") {
		t.Fatalf("%s", one)
	}
}
