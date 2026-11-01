package agent

// Language. Everything infuse writes for the model — system prompt, tool descriptions, tool outputs,
// refusals — is English (the language models follow best). The model is told to answer the user in the
// language configured in infuse. What the user sees of infuse's own messages is written in that
// language from the message catalog (msgs.go): a message is an identifier and named arguments, never
// text to be recognized afterwards.

import (
	"fmt"
	"infuse/internal/config"
	"strings"
)

// registerTranslations is a no-op kept only so i18n_pt_tools.go still compiles: removed when
// i18n_pt_tools.go is gone.
func registerTranslations(lang string, m map[string]string) {}

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

// uiLanguage is the same answer for package functions that have no session.
func uiLanguage() string {
	if l := config.LoadSettings().Language; l != "" {
		return l
	}
	return "pt-BR"
}
