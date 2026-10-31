package core

// infuse's messages are data. A message is an identifier from the catalog and named arguments; each
// language writes it from its own template ("{file} would have {lines} lines"), placing the arguments
// wherever its grammar wants them. An argument may itself be a message (a reason inside a refusal),
// written in the same language. The model always gets the English writing; the user gets their
// configured language. Nothing reads text back to find out what a message was: whoever produces a
// message records it with the tool call (or the event) it belongs to, so the user's version is built
// from the data, never from parsing words.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Msg is one message of infuse: catalog identifier plus named arguments.
type Msg struct {
	ID   string         `json:"id"`
	Args map[string]any `json:"args,omitempty"`
}

var (
	catalog          = map[string]map[string]string{} // id → language → template
	catalogConflicts []string                         // ids registered twice with different English (a test fails on them)
	catalogMu        sync.RWMutex
)

// RegisterMessages adds messages to the catalog: id → {language: template}. English is required.
func RegisterMessages(m map[string]map[string]string) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	for id, langs := range m {
		if old, ok := catalog[id]; ok && old["en"] != langs["en"] { // two catalogs gave the same id different meanings
			catalogConflicts = append(catalogConflicts, id)
		}
		if catalog[id] == nil {
			catalog[id] = map[string]string{}
		}
		for l, t := range langs {
			catalog[id][l] = t
		}
	}
}

// M builds a message: M("diag.ev_quote", "n", 1, "file", rel).
func M(id string, kv ...any) Msg {
	m := Msg{ID: id}
	if len(kv) > 0 {
		m.Args = map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m.Args[fmt.Sprint(kv[i])] = kv[i+1]
		}
	}
	return m
}

func (m Msg) Empty() bool { return m.ID == "" }

var rePlaceholder = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// Render writes the message in a language (falling back to English, then to the identifier).
func (m Msg) Render(lang string) string {
	if m.ID == "" {
		return ""
	}
	catalogMu.RLock()
	langs := catalog[m.ID]
	t := langs[LangKey(lang)]
	if t == "" {
		t = langs["en"]
	}
	catalogMu.RUnlock()
	if t == "" {
		return m.ID
	}
	return rePlaceholder.ReplaceAllStringFunc(t, func(ph string) string {
		v, ok := m.Args[ph[1:len(ph)-1]]
		if !ok {
			return ph
		}
		return renderArg(v, lang)
	})
}

// String is the model's version.
func (m Msg) String() string { return m.Render("en") }

func renderArg(v any, lang string) string {
	switch x := v.(type) {
	case Msg:
		return x.Render(lang)
	case []Msg:
		parts := make([]string, len(x))
		for i, m := range x {
			parts[i] = m.Render(lang)
		}
		return strings.Join(parts, "\n- ")
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprint(int64(x))
		}
		return fmt.Sprintf("%.1f", x)
	}
	return fmt.Sprint(v)
}

// LangKey maps a configured language to the catalog's keys ("pt-BR" → "pt-BR", "pt" → "pt-BR").
func LangKey(lang string) string {
	switch strings.ToLower(lang) {
	case "", "pt", "pt-br":
		return "pt-BR"
	case "en", "en-us", "en-gb":
		return "en"
	}
	return lang
}

// DisplayFor is the user's version of a tool output: each message infuse recorded for the call is
// written again in the user's language, in place of the exact English text it produced. Output that is
// not infuse's (a file, a command's output) is left as it is.
func DisplayFor(lang, content string, msgs []Msg) string {
	if LangKey(lang) == "en" || len(msgs) == 0 {
		return ""
	}
	// longest first: a message that contains another is replaced whole
	sort.SliceStable(msgs, func(i, j int) bool { return len(msgs[i].String()) > len(msgs[j].String()) })
	out := content
	for _, m := range msgs {
		if en := m.String(); en != "" {
			out = strings.Replace(out, en, m.Render(lang), 1)
		}
	}
	if out == content {
		return ""
	}
	return out
}

// Catalog returns a copy of the registered catalog, for validation tests and tooling.
func Catalog() map[string]map[string]string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	out := make(map[string]map[string]string, len(catalog))
	for id, langs := range catalog {
		cp := make(map[string]string, len(langs))
		for l, t := range langs {
			cp[l] = t
		}
		out[id] = cp
	}
	return out
}

// Conflicts lists ids registered twice with different meanings.
func Conflicts() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	return append([]string(nil), catalogConflicts...)
}

// PlaceholderNames returns the named arguments a template uses, sorted.
func PlaceholderNames(s string) []string {
	var out []string
	for _, m := range rePlaceholder.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}
