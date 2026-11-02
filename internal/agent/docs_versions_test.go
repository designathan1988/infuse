package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocURLVerifiesVersionShapes(t *testing.T) {
	cases := []struct {
		dep, version, want string
	}{
		{"django", "5.1.1", "https://docs.djangoproject.com/en/5.1/"},
		{"django", "4.2", "https://docs.djangoproject.com/en/4.2/"},
		{"django", "5", "https://docs.djangoproject.com/en/5.0/"},
		{"flask", "3.0.3", "https://flask.palletsprojects.com/en/3.0.x/"},
		{"pytest", "8.3.3", "https://docs.pytest.org/en/8.3.x/"},
		{"express", "4.19.2", "https://expressjs.com/en/4x/api.html"},
		{"express", "5.0.0-beta.1", "https://expressjs.com/en/5x/api.html"},
		{"react", "19.0.0", ""}, // the vendor has no versioned URL
		{"django", "", ""},      // no version known
		{"django", "next", ""},  // not a version
	}
	for _, c := range cases {
		if got := docURL(c.dep, c.version); got != c.want {
			t.Errorf("docURL(%q, %q) = %q, want %q", c.dep, c.version, got, c.want)
		}
	}
}

func TestNpmDependencyVersionsReadRanges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(
		`{"dependencies":{"express":"^4.19.2","react":"*"},"devDependencies":{"Vitest":"~2.1.1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	v := npmDependencyVersions(root)
	if v["express"] != "4.19.2" || v["react"] != "" || v["vitest"] != "2.1.1" {
		t.Fatalf("%+v", v)
	}
}

func TestPythonDependencyVersionsPinnedFromManifests(t *testing.T) {
	root := t.TempDir()
	req := "# web stack\n" +
		"django==5.1.1\n" +
		"flask[async]~=3.0.3 ; python_version >= '3.9'\n" +
		"fastapi>=0.115.0,<1.0\n" +
		"pytest\n"
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte(req), 0o644); err != nil {
		t.Fatal(err)
	}
	pyproject := "[project]\nname = \"demo\"\ndependencies = [\n    \"django==5.1.1\",\n    \"pytest==8.3.3\",\n]\n\n[tool.poetry.dependencies]\nflask = \"^3.0\"\n"
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(pyproject), 0o644); err != nil {
		t.Fatal(err)
	}
	v := pythonDependencyVersions(root)
	if v["django"] != "5.1.1" {
		t.Fatalf("django: %q", v["django"])
	}
	if v["flask"] != "3.0.3" {
		t.Fatalf("flask: %q", v["flask"])
	}
	if v["fastapi"] != "0.115.0" {
		t.Fatalf("fastapi: %q", v["fastapi"])
	}
	if v["pytest"] != "8.3.3" {
		t.Fatalf("pytest: %q", v["pytest"])
	}
}

func TestSuggestDocsCarriesTheInstalledVersionInTheURL(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("django==5.1.1\nfastapi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"express":"^4.19.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	urls := map[string]string{}
	for _, s := range SuggestDocs(root) {
		urls[s.Name] = s.URL
	}
	if urls["Django"] != "https://docs.djangoproject.com/en/5.1/" {
		t.Fatalf("Django: %q", urls["Django"])
	}
	if urls["FastAPI"] != "https://fastapi.tiangolo.com/reference/" {
		t.Fatalf("FastAPI: %q", urls["FastAPI"])
	}
	if urls["Express"] != "https://expressjs.com/en/4x/api.html" {
		t.Fatalf("Express: %q", urls["Express"])
	}
}
