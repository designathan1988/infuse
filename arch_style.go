package main

// Styles have one home: colours come from the design tokens (CSS custom properties) and look is set by
// classes in the style sheets, not by literals scattered in rules or inline styles.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reCSSVarDecl    = regexp.MustCompile(`--[\w-]+\s*:`)
	reVarFn         = regexp.MustCompile(`var\([^)]*\)`)
	reCSSTokenDecl  = regexp.MustCompile(`(--[\w-]+)\s*:\s*([^;}\n]+)`)
	reColorProp     = regexp.MustCompile(`(?i)(?:^|[;{\s"'])(?:color|background(?:-color)?|border(?:-(?:top|right|bottom|left))?(?:-color)?|outline(?:-color)?|fill|stroke|box-shadow|text-shadow|caret-color|accent-color)\s*:\s*([^;{}"'\n]*)`)
	reColorLit      = regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b|rgba?\([^)]*\)|hsla?\([^)]*\)`)
	reInlineLiteral = regexp.MustCompile(`(?i)style\s*=\s*["'][^"']*(?:#[0-9a-f]{3,8}\b|rgba?\(|hsla?\()|style=\{\{[^}]*['"](?:#[0-9a-f]{3,8}|rgba?\(|hsla?\()|\.style\.(?:color|background(?:Color)?|borderColor|fill|stroke)\s*=\s*['"\x60]`)
)

// colorLiterals lists the literal colours set on colour properties (values inside var() do not count).
func colorLiterals(code string) []string {
	var out []string
	for _, m := range reColorProp.FindAllStringSubmatch(code, -1) {
		v := reVarFn.ReplaceAllString(m[1], "")
		out = append(out, reColorLit.FindAllString(v, -1)...)
	}
	return out
}

// archStyles: new literal colours outside the token files, and inline styles with literal colours.
func archStyles(r *ArchRules, work, rel, ext, before, after string) string {
	if !r.on("styles") || len(r.TokenFiles) == 0 || contains(r.TokenFiles, rel) {
		return ""
	}
	styleLike := ext == ".css" || ext == ".scss" || ext == ".less" || ext == ".vue" || ext == ".svelte" || ext == ".html" || ext == ".astro"
	if styleLike {
		was := map[string]int{}
		for _, c := range colorLiterals(before) {
			was[strings.ToLower(c)]++
		}
		for _, c := range colorLiterals(after) {
			k := strings.ToLower(c)
			if was[k] > 0 {
				was[k]--
				continue
			}
			return fmt.Sprintf("literal color %s outside the tokens. The project's colors come from %s: use var(--…)%s. If it is a new color in the design, it goes into the tokens file first.", c, strings.Join(r.TokenFiles, ", "), tokenHint(work, r, k))
		}
	}
	if (jsLikeExts[ext] || styleLike) && len(reInlineLiteral.FindAllString(after, -1)) > len(reInlineLiteral.FindAllString(before, -1)) {
		return fmt.Sprintf("inline style with a literal color. Appearance comes from the classes in the stylesheet and colors from the tokens (%s): create or use a class; inline style only for values computed at run time.", strings.Join(r.TokenFiles, ", "))
	}
	return ""
}

// tokenHint names the tokens that already hold this colour.
func tokenHint(work string, r *ArchRules, color string) string {
	var hits []string
	for _, f := range r.TokenFiles {
		b, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		for _, m := range reCSSTokenDecl.FindAllStringSubmatch(string(b), -1) {
			if strings.EqualFold(strings.TrimSpace(m[2]), color) && !contains(hits, m[1]) {
				hits = append(hits, m[1])
			}
		}
	}
	if len(hits) == 0 {
		return ""
	}
	if len(hits) > 3 {
		hits = hits[:3]
	}
	return " — this color already exists as var(" + strings.Join(hits, "), var(") + ")"
}
