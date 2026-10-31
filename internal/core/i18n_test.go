package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	reTCall   = regexp.MustCompile(`\bt\('([^']*)'`)
	reDataI18 = regexp.MustCompile(`data-i18n(?:-title|-placeholder)?="([^"]+)"`)
	reArg     = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// Every message has both languages, and both place the same named arguments.
func TestCatalogIsComplete(t *testing.T) {
	var catalog map[string]map[string]string
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	for id, langs := range catalog {
		if langs["en"] == "" || langs["pt-BR"] == "" {
			t.Errorf("%s: missing a language", id)
			continue
		}
		if a, b := placeholders(langs["en"]), placeholders(langs["pt-BR"]); strings.Join(a, ",") != strings.Join(b, ",") {
			t.Errorf("%s: placeholders differ: %v vs %v", id, a, b)
		}
	}
}

func placeholders(s string) []string {
	var out []string
	for _, m := range reArg.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// Every literal translation call uses a catalog id, and every id it names exists.
func TestUIMessagesUseCatalogIDs(t *testing.T) {
	var catalog map[string]map[string]string
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "ui")
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "vendor" {
			return filepath.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".html")) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		for _, m := range reTCall.FindAllStringSubmatch(text, -1) {
			id := m[1]
			if !strings.HasPrefix(id, "ui.") {
				t.Errorf("%s: t(%q) is not a catalog id", file, id)
				continue
			}
			if _, ok := catalog[id]; !ok {
				t.Errorf("%s: %s is missing from catalog.json", file, id)
			}
		}
		for _, m := range reDataI18.FindAllStringSubmatch(text, -1) {
			if _, ok := catalog[m[1]]; !ok {
				t.Errorf("%s: %s is missing from catalog.json", file, m[1])
			}
		}
	}
}
