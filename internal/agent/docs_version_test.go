package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDocFixture(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func suggestionsByName(root string) map[string]docSuggestion {
	out := map[string]docSuggestion{}
	for _, s := range SuggestDocs(root) {
		out[s.Name] = s
	}
	return out
}

// Django, pytest and Flask publish stable versioned documentation; the suggested URL follows the
// version the project pins. FastAPI publishes one current site, so it keeps the stable URL and the
// reason states the version.
func TestSuggestDocsPinsPythonVersions(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeDocFixture(t, root, "requirements.txt", "Django==5.0.6\npytest~=8.3\nflask>=3.0,<4\n")
	writeDocFixture(t, root, "pyproject.toml", "[project]\ndependencies = [\"fastapi==0.115.0\"]\n")
	writeDocFixture(t, root, "package.json", `{"dependencies":{"react":"19.0.0","express":"^4.19.2"}}`)
	got := suggestionsByName(root)

	if d := got["Django"]; d.URL != "https://docs.djangoproject.com/en/5.0/" || !strings.Contains(d.Why, "5.0.6") {
		t.Fatalf("django: %+v", d)
	}
	if p := got["pytest"]; p.URL != "https://docs.pytest.org/en/8.3.x/" || !strings.Contains(p.Why, "8.3") {
		t.Fatalf("pytest: %+v", p)
	}
	if f := got["Flask"]; f.URL != "https://flask.palletsprojects.com/en/3.0.x/" || !strings.Contains(f.Why, "3.0") {
		t.Fatalf("flask: %+v", f)
	}
	if f := got["FastAPI"]; f.URL != "https://fastapi.tiangolo.com/reference/" || !strings.Contains(f.Why, "0.115.0") {
		t.Fatalf("fastapi: %+v", f)
	}
	if e := got["Express"]; e.URL != "https://expressjs.com/en/4x/api.html" || !strings.Contains(e.Why, "4.19.2") {
		t.Fatalf("express: %+v", e)
	}
	if r := got["React"]; r.URL != "https://react.dev/reference/react" || !strings.Contains(r.Why, "19.0.0") {
		t.Fatalf("react: %+v", r)
	}
}

// No pin: the stable URL is still the right suggestion.
func TestSuggestDocsFallsBackWithoutVersions(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeDocFixture(t, root, "requirements.txt", "Django\npytest\n")
	got := suggestionsByName(root)
	if d := got["Django"]; d.URL != "https://docs.djangoproject.com/en/stable/" {
		t.Fatalf("django: %+v", d)
	}
	if p := got["pytest"]; p.URL != "https://docs.pytest.org/en/stable/reference/reference.html" {
		t.Fatalf("pytest: %+v", p)
	}
}

// Poetry ranges use the lower bound when there is no exact pin.
func TestSuggestDocsReadsPoetryVersions(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeDocFixture(t, root, "pyproject.toml", "[tool.poetry.dependencies]\ndjango = \"^4.2\"\npytest = \"8.3.2\"\n")
	got := suggestionsByName(root)
	if d := got["Django"]; d.URL != "https://docs.djangoproject.com/en/4.2/" {
		t.Fatalf("django: %+v", d)
	}
	if p := got["pytest"]; p.URL != "https://docs.pytest.org/en/8.3.x/" {
		t.Fatalf("pytest: %+v", p)
	}
}

// An exact pin wins over a lower bound, whichever line it is on.
func TestSuggestDocsPrefersExactPin(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeDocFixture(t, root, "requirements.txt", "pytest>=7.0\npytest==8.3.2\n")
	if p := suggestionsByName(root)["pytest"]; p.URL != "https://docs.pytest.org/en/8.3.x/" {
		t.Fatalf("pytest: %+v", p)
	}
}

// The same dependency in two manifests produces one suggestion for the same URL.
func TestSuggestDocsDeduplicatesByURL(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeDocFixture(t, root, "requirements.txt", "Django==5.0.6\n")
	writeDocFixture(t, root, "pyproject.toml", "[project]\ndependencies = [\"django==5.0.6\"]\n")
	n := 0
	for _, s := range SuggestDocs(root) {
		if s.URL == "https://docs.djangoproject.com/en/5.0/" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("django suggested %d times", n)
	}
}
