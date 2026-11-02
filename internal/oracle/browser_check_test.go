package oracle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infuse/internal/browser"
)

// serverCommand starts this test binary as the static file server of the snapshot (TestMain).
func serverCommand(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(serveEnv, "1")
	return "'" + filepath.ToSlash(exe) + "'"
}

func browserCheck(url, selector string) OracleCheck {
	c := OracleCheck{ID: "page", Kind: "browser", URL: url}
	c.Expect.Selector = selector
	return c
}

func TestLintRefusesABrowserCheckThatCannotFail(t *testing.T) {
	bad := []OracleCheck{
		browserCheck("", "#x"),
		browserCheck("file:///etc/passwd", "#x"),
		browserCheck("http://localhost:1/", ""),
		browserCheck("http://localhost:1/", "body"),
		browserCheck("http://localhost:1/", "html"),
	}
	step := browserCheck("http://localhost:1/", "#x")
	step.Steps = []browser.ScenarioStep{{Action: "click"}}
	bad = append(bad, step)
	press := browserCheck("http://localhost:1/", "#x")
	press.Steps = []browser.ScenarioStep{{Action: "press"}}
	bad = append(bad, press)
	unknown := browserCheck("http://localhost:1/", "#x")
	unknown.Steps = []browser.ScenarioStep{{Action: "eval", Selector: "#x"}}
	bad = append(bad, unknown)
	for _, c := range bad {
		if err := LintSpec(OracleSpec{Checks: []OracleCheck{c}}); err == nil {
			t.Fatalf("accepted: %+v", c)
		}
	}
	good := browserCheck("http://localhost:1/", "#x")
	good.Steps = []browser.ScenarioStep{{Action: "click", Selector: "#go"}, {Action: "type", Selector: "#in", Text: "a"}, {Action: "press", Text: "Enter"}}
	if err := LintSpec(OracleSpec{Checks: []OracleCheck{good}}); err != nil {
		t.Fatal(err)
	}
}

// Two scenarios on one address are two criteria, not a duplicate.
func TestLintTellsBrowserScenariosApart(t *testing.T) {
	a, b := browserCheck("http://localhost:1/", "#a"), browserCheck("http://localhost:1/", "#b")
	b.ID = "other"
	if err := LintSpec(OracleSpec{Checks: []OracleCheck{a, b}}); err != nil {
		t.Fatal(err)
	}
	b.Expect.Selector = "#a"
	if err := LintSpec(OracleSpec{Checks: []OracleCheck{a, b}}); err == nil {
		t.Fatal("the same scenario twice is a duplicate")
	}
}

func TestBrowserCheckWithoutServerIsRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"p","kind":"browser","url":"http://127.0.0.1:1/","expect":{"selector":"#x"}}]}`)
	if v := run(t, root); v.Verdict != "erro" || len(v.Checks) != 0 {
		t.Fatalf("%+v", v)
	}
}

// No browser on the machine must degrade to an error of the run: never a pass, and never a failure
// charged to the project.
func TestNoBrowserIsAnErrorNotAPass(t *testing.T) {
	testHome(t)
	old := runScenario
	runScenario = func(context.Context, browser.Scenario) (browser.ScenarioResult, error) {
		return browser.ScenarioResult{}, fmt.Errorf("%w: not installed", browser.ErrNoBrowser)
	}
	t.Cleanup(func() { runScenario = old })
	root := t.TempDir()
	port := freePort(t)
	mustWrite(t, filepath.Join(root, "index.html"), "<p>x</p>\n")
	spec := OracleSpec{Server: serverCommand(t)}
	spec.Checks = []OracleCheck{browserCheck(fmt.Sprintf("http://127.0.0.1:%d/", port), "#x")}
	b, _ := json.Marshal(spec)
	writeSpec(t, root, string(b))
	v := run(t, root)
	if v.Verdict != "erro" || len(v.Checks) != 1 || v.Checks[0].Passed || !v.Checks[0].Errored {
		t.Fatalf("%+v", v)
	}
}

func TestBrowserAssertionsJudgeTheFinalPage(t *testing.T) {
	c := browserCheck("http://x/", "#ok")
	c.Expect.Contains = "hello"
	if ok, _ := browserAssert(c, browser.ScenarioResult{Status: 200, Found: true, Text: "hello"}); !ok {
		t.Fatal("a matching page must pass")
	}
	for name, res := range map[string]browser.ScenarioResult{
		"status":   {Status: 500, Found: true, Text: "hello"},
		"selector": {Status: 200, Found: false, Text: "hello"},
		"text":     {Status: 200, Found: true, Text: "bye"},
		"nothing":  {},
	} {
		if ok, _ := browserAssert(c, res); ok {
			t.Fatalf("%s: passed", name)
		}
	}
}

const testPage = `<!doctype html><title>t</title><input id="name"><button id="go" onclick="document.getElementById('out').textContent='hello '+document.getElementById('name').value">go</button><div id="out"></div>`

// The whole adapter against a real browser and a server started from the snapshot: the page is served
// by a contained child, the steps run, the text is asserted and the screenshot is kept.
func TestBrowserCheckInARealBrowser(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	port := freePort(t)
	mustWrite(t, filepath.Join(root, "index.html"), testPage)
	c := browserCheck(fmt.Sprintf("http://127.0.0.1:%d/index.html", port), "#out")
	c.Steps = []browser.ScenarioStep{{Action: "type", Selector: "#name", Text: "ana"}, {Action: "click", Selector: "#go"}}
	c.Expect.Contains = "hello ana"
	spec := OracleSpec{Server: serverCommand(t), Checks: []OracleCheck{c}}
	b, _ := json.Marshal(spec)
	writeSpec(t, root, string(b))
	v := run(t, root)
	if v.Verdict == "erro" && len(v.Checks) == 1 && v.Checks[0].Errored {
		t.Skip("no browser on this machine: " + v.Checks[0].Detail)
	}
	if v.Verdict != "aprovado" {
		t.Fatalf("%+v", v)
	}
	if v.ServerPID == 0 || v.Checks[0].Screenshot == "" {
		t.Fatalf("%+v", v)
	}
	png, err := os.ReadFile(filepath.Join(EvidenceDir(root), v.Run, v.Checks[0].Screenshot))
	if err != nil || len(png) < 8 || string(png[1:4]) != "PNG" {
		t.Fatalf("the screenshot is not evidence: %v", err)
	}
	log, _ := os.ReadFile(filepath.Join(EvidenceDir(root), v.Run, v.Checks[0].Evidence))
	if !strings.Contains(string(log), "hello ana") {
		t.Fatalf("%s", log)
	}
}

// An HTTP check against the same server proves the ownership check accepts the genuine listener, which
// here is a grandchild of the executor behind the sandbox wrapper and a shell.
func TestServerStartedFromTheSnapshotIsAccepted(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	port := freePort(t)
	mustWrite(t, filepath.Join(root, "index.html"), "from the snapshot\n")
	spec := OracleSpec{Server: serverCommand(t)}
	c := OracleCheck{ID: "page", Kind: "http", URL: fmt.Sprintf("http://localhost:%d/index.html", port)}
	c.Expect.Contains = "from the snapshot"
	spec.Checks = []OracleCheck{c}
	b, _ := json.Marshal(spec)
	writeSpec(t, root, string(b))
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("%+v", v)
	}
	if _, err := portFreeAfterStop(port); err != nil {
		t.Fatal(err)
	}
}

// portFreeAfterStop confirms the server's tree ended with the run.
func portFreeAfterStop(port int) (bool, error) {
	if err := portFree(fmt.Sprintf("http://127.0.0.1:%d/", port)); err != nil {
		return false, errors.New("the server outlived the run: " + err.Error())
	}
	return true, nil
}

func TestWebProjectsGetABrowserSuggestion(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"scripts":{"dev":"vite"},"devDependencies":{"vite":"^5"}}`)
	var got *CheckSuggestion
	for _, s := range SuggestChecks(root) {
		if s.Check.Kind == "browser" {
			got = &s
		}
	}
	if got == nil {
		t.Fatal("no browser suggestion for a vite project")
	}
	if got.Server == "" || !strings.Contains(got.Server, "dev") {
		t.Fatalf("the suggestion does not say how to serve the page: %+v", got)
	}
	if err := LintSpec(OracleSpec{Checks: []OracleCheck{got.Check}}); err != nil {
		t.Fatal(err)
	}
}
