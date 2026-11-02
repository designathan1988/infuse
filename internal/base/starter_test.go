package base

import (
	"infuse/internal/config"
	"infuse/internal/oracle"
	"infuse/internal/projectmap"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "infuse-base-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("INFUSE_HOME", home)
	config.SecretBackend = config.MemSecrets{}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// Every base of the SPEC exists and defines each element of the contract.
func TestAllSevenBasesCarryTheWholeContract(t *testing.T) {
	want := []string{"api", "web", "cli", "interactive", "library", "game", "service"}
	names := map[string]Base{}
	for _, b := range Catalog() {
		names[b.Name] = b
	}
	for _, n := range want {
		b, ok := names[n]
		if !ok {
			t.Fatalf("base %s is missing", n)
		}
		if len(b.Model) == 0 || len(b.Model[0].States) == 0 || b.Model[0].Owner == "" {
			t.Errorf("%s: no canonical model with owner and states", n)
		}
		if len(b.Layers) == 0 || len(b.Registries) == 0 || b.Config == "" || b.Persistence == "" || b.Observability == "" || b.Surface == "" || b.DataDir == "" || len(b.Oracle) == 0 {
			t.Errorf("%s: incomplete contract: %+v", n, b)
		}
		contract := b.Contract()
		for _, part := range []string{"Layers", b.Model[0].Owner, b.Persistence, b.Observability, b.Registries[0].Path} {
			if !strings.Contains(contract, part) {
				t.Errorf("%s: the prompt's contract omits %q", n, part)
			}
		}
		if strings.Contains(contract, "{{") {
			t.Errorf("%s: an unfilled placeholder in the contract", n)
		}
	}
}

// Each scaffold is a working starter: its own smoke test passes, by the toolchain of its language,
// with no network, and the starter breaks no rule of its own base.
func TestEveryScaffoldRunsItsSmokeTest(t *testing.T) {
	for _, b := range Catalog() {
		t.Run(b.Name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "my-"+b.Name)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := Scaffold(root, b.Name); err != nil {
				t.Fatal(err)
			}
			assertStarter(t, root, b)
			runSmoke(t, root, b)
		})
	}
}

func assertStarter(t *testing.T, root string, b Base) {
	t.Helper()
	for _, d := range append([]string{b.DataDir}, b.Folders...) {
		if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(d))); err != nil || !st.IsDir() {
			t.Errorf("declared folder %s missing", d)
		}
	}
	m := projectmap.LoadMap(root)
	if m == nil || len(m.Files) < 5 {
		t.Fatalf("the map was not filled from the scaffold: %+v", m)
	}
	for _, x := range m.Drift {
		if x.Kind != "intended-only" {
			t.Errorf("the starter breaks its own base: %+v", x)
		}
	}
	if len(m.Data) == 0 || len(m.Examples) == 0 {
		t.Errorf("levels empty: data=%d examples=%d", len(m.Data), len(m.Examples))
	}
	spec, err := oracle.LoadOracleSpec(root)
	if err != nil || len(spec.Checks) == 0 {
		t.Fatalf("no default oracle: %v", err)
	}
	if err := oracle.LintSpec(spec); err != nil {
		t.Fatal(err)
	}
	if !projectmap.ContextFilesEnabled(root) {
		t.Error("context files are on for a scaffold")
	}
	if b, err := os.ReadFile(filepath.Join(root, "CLAUDE.md")); err != nil || !strings.Contains(string(b), "infuse:begin map") {
		t.Errorf("no generated context at the root: %v", err)
	}
}

func runSmoke(t *testing.T, root string, b Base) {
	t.Helper()
	tool, args := "go", []string{"test", "./..."}
	if b.Language == "js" {
		tool, args = "node", []string{"--test", "tests/smoke.test.js"}
	}
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s not installed", tool)
	}
	cmd := exec.Command(tool, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", tool, args, err, out)
	}
	if b.Language == "go" {
		vet := exec.Command("go", "vet", "./...")
		vet.Dir = root
		if out, err := vet.CombinedOutput(); err != nil {
			t.Fatalf("vet: %v\n%s", err, out)
		}
	}
}

// A file that exists is never touched: not a source file, not base.md, not the user's criteria.
func TestScaffoldNeverTouchesExistingFiles(t *testing.T) {
	root := t.TempDir()
	mine := map[string]string{
		"docs/architecture/base.md":  "my document\n",
		"internal/domain/model.go":   "package domain\n// mine\n",
		"go.mod":                     "module mine\n",
		".infuse/oracle/checks.json": `{"checks":[{"id":"mine","kind":"file","path":"go.mod","expect":{"exists":true}}]}`,
	}
	for rel, body := range mine {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Scaffold(root, "api"); err != nil {
		t.Fatal(err)
	}
	for rel, body := range mine {
		if got, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); string(got) != body {
			t.Errorf("%s was overwritten: %q", rel, got)
		}
	}
}

// A different base over a project that already chose one is refused, not silently replaced.
func TestScaffoldKeepsTheProjectsChoice(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, "web", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, "api"); err == nil {
		t.Fatal("a second base replaced the project's choice")
	}
	if b := Load(root); b == nil || b.Name != "web" {
		t.Fatalf("%+v", b)
	}
}

// The scaffold pre-fills the map with the starter: roles on every folder, intended only where the
// starter wrote nothing.
func TestScaffoldPrefillsTheMapFromTheStarter(t *testing.T) {
	root := t.TempDir()
	b, err := Scaffold(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	m := projectmap.LoadOrBuildMap(root)
	for _, f := range b.Layout {
		d := m.Dirs[f.Path]
		if d == nil || d.Role != f.Role {
			t.Fatalf("folder %s: %+v", f.Path, d)
		}
	}
	if d := m.Dirs["src/core"]; d.Intended || len(d.Files) == 0 {
		t.Fatalf("src/core holds the starter: %+v", d)
	}
	if d := m.Dirs["public"]; !d.Intended {
		t.Fatalf("public has no code yet: %+v", d)
	}
	if !strings.Contains(projectmap.DescribeDir(m, "src/pages"), "role:") {
		t.Fatal("describe does not state the role")
	}
}

// Layers bind before disk: an import the base forbids is found in the text about to be written.
func TestLayerRulesCheckTheContentBeingWritten(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, "api"); err != nil {
		t.Fatal(err)
	}
	module, _ := projectNames(root)
	bad := "package domain\n\nimport \"" + module + "/internal/api\"\n\nvar _ = api.Handler\n"
	if rule, _ := CheckLayers(root, "internal/domain/bad.go", bad); !strings.Contains(rule, "internal/domain may not import internal/api") {
		t.Fatalf("rule: %q", rule)
	}
	good := "package api\n\nimport \"" + module + "/internal/domain\"\n\nvar _ domain.Item\n"
	if rule, _ := CheckLayers(root, "internal/api/ok.go", good); rule != "" {
		t.Fatalf("an allowed import was refused: %q", rule)
	}
	web := t.TempDir()
	if _, err := Scaffold(web, "web"); err != nil {
		t.Fatal(err)
	}
	if rule, _ := CheckLayers(web, "src/core/x.js", "import { a } from '../pages/home.js';\n"); rule == "" {
		t.Fatal("core reaching up to a page passed")
	}
}

// The map keeps the base's limit as its hotspot threshold.
func TestHotspotThresholdComesFromTheBase(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, "cli", nil); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "internal", "core"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "internal", "core", "big.go"), []byte("package core\n"+strings.Repeat("// x\n", 500)), 0o644)
	m := projectmap.BuildMap(root)
	if len(m.Hotspots) != 1 || m.HotspotLines != 400 {
		t.Fatalf("%+v", m.Hotspots)
	}
}
