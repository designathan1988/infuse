package main

// Language. Everything infuse writes for the model — system prompt, tool descriptions, tool outputs,
// refusals — is English (the language models follow best). The model is told to answer the user in the
// language configured in infuse. What the user sees of infuse's own messages is shown in that language:
// every message is written once, in English, as a format string (the msgid, as gettext does), and each
// UI language registers its translations of those strings. localize recognizes a formatted message by
// its template and rebuilds it in the user's language; anything without a translation stays English.

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	uiTranslations = map[string]map[string]string{} // language → English format → translated format
	compiledMsgs   = map[string][]compiledMsg{}
	i18nMu         sync.Mutex
)

type compiledMsg struct {
	re    *regexp.Regexp
	trans string
}

// registerTranslations adds a language's translations (called from init of the i18n_*.go files).
func registerTranslations(lang string, m map[string]string) {
	i18nMu.Lock()
	defer i18nMu.Unlock()
	if uiTranslations[lang] == nil {
		uiTranslations[lang] = map[string]string{}
	}
	for k, v := range m {
		uiTranslations[lang][k] = v
	}
	delete(compiledMsgs, lang)
}

var reVerb = regexp.MustCompile(`%[-+# 0]*(?:\d+|\*)?(?:\.\d+)?[vsdqfgxXtTc%]`)

// templateRegex turns a format string into a pattern that captures each argument.
func templateRegex(format string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`(?s)^`)
	last := 0
	for _, loc := range reVerb.FindAllStringIndex(format, -1) {
		b.WriteString(regexp.QuoteMeta(format[last:loc[0]]))
		if format[loc[0]:loc[1]] == "%%" {
			b.WriteString("%")
		} else {
			b.WriteString(`(.*?)`)
		}
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(format[last:]))
	b.WriteString(`$`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}

// fillTemplate puts the captured arguments into the translated format, in order.
func fillTemplate(format string, args []string) string {
	i := 0
	return reVerb.ReplaceAllStringFunc(format, func(v string) string {
		if v == "%%" {
			return "%"
		}
		if i < len(args) {
			i++
			return args[i-1]
		}
		return v
	})
}

// localize shows one of infuse's English messages in lang; paragraphs are translated one by one, and
// text with no translation (tool output, the project's own content) is kept as it is.
func localize(lang, text string) string {
	if lang == "" || strings.HasPrefix(lang, "en") || text == "" {
		return text
	}
	i18nMu.Lock()
	msgs, ok := compiledMsgs[lang]
	if !ok {
		for en, tr := range uiTranslations[lang] {
			if re := templateRegex(en); re != nil {
				msgs = append(msgs, compiledMsg{re, tr})
			}
		}
		compiledMsgs[lang] = msgs
	}
	i18nMu.Unlock()
	var one func(s string, depth int) (string, bool)
	one = func(s string, depth int) (string, bool) {
		for _, m := range msgs {
			if sub := m.re.FindStringSubmatch(s); sub != nil {
				args := sub[1:]
				if depth < 3 { // a message inside a message (a reason inside a refusal) is translated too
					args = append([]string{}, args...)
					for i, a := range args {
						if t, ok := one(a, depth+1); ok {
							args[i] = t
						}
					}
				}
				return fillTemplate(m.trans, args), true
			}
		}
		return s, false
	}
	if t, ok := one(text, 0); ok {
		return t
	}
	parts := strings.Split(text, "\n")
	changed := false
	for i, p := range parts {
		if t, ok := one(p, 0); ok {
			parts[i], changed = t, true
		}
	}
	if !changed {
		return text
	}
	return strings.Join(parts, "\n")
}

// displayIfLocalized is the user's-language version of text, or "" when it reads the same.
func displayIfLocalized(lang, text string) string {
	if d := localize(lang, text); d != text {
		return d
	}
	return ""
}

// languageName is how the model is told which language to answer in.
func languageName(code string) string {
	switch strings.ToLower(code) {
	case "pt-br", "pt", "":
		return "Brazilian Portuguese (pt-BR)"
	case "pt-pt":
		return "European Portuguese (pt-PT)"
	case "en", "en-us":
		return "English"
	case "es":
		return "Spanish"
	}
	return code
}

// responseLanguageRule is the system prompt line that fixes the language of every reply.
func responseLanguageRule(code string) string {
	return fmt.Sprintf("Write every message to the user — answers, plans, reports, questions, summaries — in %s, the language configured in infuse, even when the request, the code or these instructions are in another language. Code, identifiers, comments and commit messages follow the project's own conventions.", languageName(code))
}

// uiLang is the language the user reads infuse in.
func (s *Session) uiLang() string {
	if s.settings.Language != "" {
		return s.settings.Language
	}
	return "pt-BR"
}
