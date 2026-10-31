package main

// Architecture rules read from the code itself: god functions (length, branches, nesting), literal
// branches that should be a table, global state outside the store, constants defined twice.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// stripCode blanks comments and string contents (newlines kept), so braces and keywords can be counted.
func stripCode(code, ext string) string {
	b := []byte(code)
	out := make([]byte, len(b))
	py := ext == ".py"
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case !py && c == '/' && i+1 < len(b) && b[i+1] == '/', py && c == '#':
			for i < len(b) && b[i] != '\n' {
				out[i] = ' '
				i++
			}
		case !py && c == '/' && i+1 < len(b) && b[i+1] == '*':
			for i < len(b) && !(b[i] == '*' && i+1 < len(b) && b[i+1] == '/') {
				out[i] = blankKeepNL(b[i])
				i++
			}
			for k := 0; k < 2 && i < len(b); k++ {
				out[i] = ' '
				i++
			}
		case c == '"' || c == '\'' || c == '`':
			q := c
			out[i] = c
			i++
			for i < len(b) && b[i] != q {
				if b[i] == '\\' && i+1 < len(b) {
					out[i], out[i+1] = ' ', blankKeepNL(b[i+1])
					i += 2
					continue
				}
				if b[i] == '\n' && q != '`' && !py {
					break
				}
				out[i] = blankKeepNL(b[i])
				i++
			}
			if i < len(b) {
				out[i] = b[i]
				i++
			}
		default:
			out[i] = c
			i++
		}
	}
	return string(out)
}

func blankKeepNL(c byte) byte {
	if c == '\n' {
		return '\n'
	}
	return ' '
}

type funcMetric struct {
	name                   string
	line, lines, cyc, nest int
}

var (
	reJSFunc     = regexp.MustCompile(`(?m)(?:\bfunction\s+([A-Za-z_$][\w$]*)\s*\(|\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:function\b|\([^()]*\)\s*(?::\s*[^=]+)?=>|[A-Za-z_$][\w$]*\s*=>)|^[ \t]*(?:public\s+|private\s+|protected\s+|static\s+|async\s+)*([A-Za-z_$][\w$]*)\s*\([^()]*\)\s*(?::\s*[^{;]+)?\{)`)
	reGoFunc     = regexp.MustCompile(`(?m)^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*[\[(]`)
	rePyFunc     = regexp.MustCompile(`(?m)^([ \t]*)(?:async\s+)?def\s+(\w+)\s*\(`)
	reBranchC    = regexp.MustCompile(`\b(?:if|for|while|case|catch)\b|&&|\|\||\?\?|\?[^.?:]`)
	reBranchGo   = regexp.MustCompile(`\b(?:if|for|case)\b|&&|\|\|`)
	reBranchPy   = regexp.MustCompile(`\b(?:if|elif|for|while|except|and|or)\b`)
	notFuncNames = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true, "return": true, "function": true, "else": true, "do": true, "with": true}
)

// functionMetrics measures every function of a file.
func functionMetrics(code, ext string) []funcMetric {
	s := stripCode(code, ext)
	var out []funcMetric
	lineOf := func(pos int) int { return strings.Count(s[:pos], "\n") + 1 }
	switch {
	case ext == ".py":
		lines := strings.Split(s, "\n")
		for _, m := range rePyFunc.FindAllStringSubmatchIndex(s, -1) {
			indent := len(s[m[2]:m[3]])
			start := lineOf(m[0]) - 1
			end, nest := start, 0
			for k := start + 1; k < len(lines); k++ {
				t := strings.TrimLeft(lines[k], " \t")
				if t == "" {
					continue
				}
				in := len(lines[k]) - len(t)
				if in <= indent {
					break
				}
				end = k
				if d := (in - indent) / 4; d > nest {
					nest = d
				}
			}
			body := strings.Join(lines[start:end+1], "\n")
			out = append(out, funcMetric{name: s[m[4]:m[5]], line: start + 1, lines: end - start + 1, cyc: 1 + len(reBranchPy.FindAllString(body, -1)), nest: max(0, nest-1)})
		}
		return out
	case ext == ".go" || jsLikeExts[ext] || ext == ".java" || ext == ".cs" || ext == ".rs" || ext == ".php" || ext == ".kt":
		re, branch := reJSFunc, reBranchC
		if ext == ".go" {
			re, branch = reGoFunc, reBranchGo
		}
		for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
			name := ""
			for g := 2; g+1 < len(m); g += 2 {
				if m[g] >= 0 {
					name = s[m[g]:m[g+1]]
					break
				}
			}
			if name == "" || notFuncNames[name] {
				continue
			}
			open := strings.IndexByte(s[m[1]-1:], '{')
			if open < 0 || strings.Count(s[m[1]-1:m[1]-1+open], "\n") > 3 { // an arrow without a block, or no body
				continue
			}
			open += m[1] - 1
			depth, maxDepth, end := 0, 0, -1
			for k := open; k < len(s); k++ {
				switch s[k] {
				case '{':
					depth++
					maxDepth = max(maxDepth, depth)
				case '}':
					depth--
				}
				if depth == 0 {
					end = k
					break
				}
			}
			if end < 0 {
				continue
			}
			body := s[open : end+1]
			out = append(out, funcMetric{name: name, line: lineOf(m[0]), lines: lineOf(end) - lineOf(m[0]) + 1, cyc: 1 + len(branch.FindAllString(body, -1)), nest: maxDepth - 1})
		}
	}
	return out
}

// archFunctions: a function may not become a god function — too long, too many branches, too deep.
// A function that already was keeps its size, but may not get worse.
func archFunctions(r *ArchRules, rel, ext, before, after string) string {
	if !r.on("functions") {
		return ""
	}
	old := map[string]funcMetric{}
	for _, f := range functionMetrics(before, ext) {
		old[f.name] = f
	}
	for _, f := range functionMetrics(after, ext) {
		o, had := old[f.name]
		bad := func(v, ov, lim int) bool { return v > lim && (!had || v > ov) }
		switch {
		case bad(f.lines, o.lines, r.FuncLines):
			return fmt.Sprintf("a função %s (%s:%d) ficaria com %d linhas (limite %d). Uma função faz uma coisa: extraia as etapas em funções com nomes que digam o que fazem.", f.name, rel, f.line, f.lines, r.FuncLines)
		case bad(f.cyc, o.cyc, r.Cyclomatic):
			return fmt.Sprintf("a função %s (%s:%d) ficaria com complexidade %d (limite %d: caminhos por if/for/case/&&/||). Separe as decisões em funções menores, ou troque cadeias de condições por uma tabela de dados.", f.name, rel, f.line, f.cyc, r.Cyclomatic)
		case bad(f.nest, o.nest, r.Nesting):
			return fmt.Sprintf("a função %s (%s:%d) ficaria com %d níveis de aninhamento (limite %d). Use retornos antecipados e extraia os blocos internos.", f.name, rel, f.line, f.nest, r.Nesting)
		}
	}
	return ""
}

var (
	reCaseArm = regexp.MustCompile(`^\s*case\s+(?:'[^']*'|"[^"]*"|-?\d[\w.]*|[A-Z][\w.]*)\s*:\s*(?:return\b|[\w.\[\]]+\s*=[^=]|$)`)
	reIfArm   = regexp.MustCompile(`^\s*(?:\}\s*)?(?:else\s+)?(?:if|elif)\s*\(?\s*([\w.\[\]]+)\s*(?:===?|==)\s*(?:'[^']*'|"[^"]*"|-?\d[\w.]*)\s*\)?\s*[:{]?\s*(?:return\b|[\w.\[\]]+\s*=[^=]|$)`)
	reFiller  = regexp.MustCompile(`^\s*(?:break;?|\}|return\b.*|[\w.\[\]]+\s*=[^=].*|)\s*$`)
)

// longestLiteralChain is the longest run of branches that map a literal to a value (switch arms or an
// if/else-if chain over the same variable).
func longestLiteralChain(code string) int {
	best, run, lastVar := 0, 0, ""
	for _, l := range strings.Split(code, "\n") {
		if reCaseArm.MatchString(l) {
			run++
		} else if m := reIfArm.FindStringSubmatch(l); m != nil {
			if m[1] != lastVar {
				run = 0
			}
			lastVar = m[1]
			run++
		} else if !reFiller.MatchString(l) {
			run, lastVar = 0, ""
		}
		best = max(best, run)
	}
	return best
}

// archTables: many literal branches are data, not logic (Code Complete ch. 18, table-driven methods).
func archTables(r *ArchRules, ext, before, after string) string {
	if !r.on("tables") || !codeExts[ext] || ext == ".css" || ext == ".html" {
		return ""
	}
	if n := longestLiteralChain(after); n >= r.TableArms && n > longestLiteralChain(before) {
		return fmt.Sprintf("%d ramos seguidos que só associam um valor literal a outro (switch/case ou if/else if). Isso é uma tabela: declare um mapa ou objeto de dados (chave → valor) e consulte-o — um lugar para os dados, uma linha de lógica.", n)
	}
	return ""
}

var (
	reStoreCreate = regexp.MustCompile(`\b(?:createStore|configureStore|defineStore|createSlice)\s*\(|\bcreate(?:<[^>]*>)?\(\s*\(?\s*set\b`)
	reGlobalWrite = regexp.MustCompile(`(?m)\b(?:window|globalThis|self)\.[A-Za-z_$][\w$]*\s*=[^=]|^\s*export\s+let\s`)
)

// archState: one store; state is not scattered in globals (Redux style guide: one store, no mutation
// outside it).
func archState(r *ArchRules, rel, ext, before, after string) string {
	if !r.on("state") || !jsLikeExts[ext] {
		return ""
	}
	if reStoreCreate.MatchString(after) && !reStoreCreate.MatchString(before) && len(r.StoreFiles) > 0 && !contains(r.StoreFiles, rel) {
		return "este arquivo cria uma nova store, mas o projeto já tem a sua em " + strings.Join(r.StoreFiles, ", ") + ". Uma fonte de verdade para o estado: acrescente o estado novo lá (um slice ou módulo dela)."
	}
	if contains(r.StoreFiles, rel) {
		return ""
	}
	if n := len(reGlobalWrite.FindAllString(stripCode(after, ext), -1)); n > len(reGlobalWrite.FindAllString(stripCode(before, ext), -1)) {
		where := "um módulo de estado"
		if len(r.StoreFiles) > 0 {
			where = strings.Join(r.StoreFiles, ", ")
		}
		return "estado global solto (window.x = …, globalThis.x = … ou export let). O estado vive em " + where + " e muda só por ele; os outros módulos leem e pedem mudanças."
	}
	return ""
}

var (
	reConstJS = regexp.MustCompile(`(?m)^(?:export\s+)?const\s+([A-Z][A-Z0-9_]{2,})\s*(?::[^=]+)?=\s*['"\d\[{-]`)
	reConstPy = regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]{2,})\s*(?::[^=]+)?=\s*['"\d\[{-]`)
	reConstGo = regexp.MustCompile(`(?m)^\s*(?:const\s+)?([A-Z][A-Z0-9_]{2,})\s*(?:[\w.]+\s*)?=\s*['"\d\x60-]`)
)

func constNames(code, ext string) map[string]bool {
	re := reConstJS
	switch {
	case ext == ".py":
		re = reConstPy
	case ext == ".go":
		re = reConstGo
	case !jsLikeExts[ext]:
		return nil
	}
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(code, -1) {
		out[m[1]] = true
	}
	return out
}

// archConstants: a constant has one home. Defining again a name another file already defines makes two
// sources of truth that drift apart.
func archConstants(r *ArchRules, work, rel, ext, before, after string) string {
	if !r.on("constants") {
		return ""
	}
	added := constNames(after, ext)
	for n := range constNames(before, ext) {
		delete(added, n)
	}
	if len(added) == 0 {
		return ""
	}
	found := ""
	walkProjectFiles(work, func(orel, abs string, size int64) {
		if found != "" || orel == rel || size > 300<<10 || sameFamily(ext, strings.ToLower(filepath.Ext(orel))) == false {
			return
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		for n := range constNames(string(b), ext) {
			if added[n] {
				found = fmt.Sprintf("a constante %s já está definida em %s. Fonte única da verdade: importe de lá em vez de definir outra (se os valores devem ser diferentes, os nomes também devem).", n, orel)
				return
			}
		}
	})
	return found
}

func sameFamily(a, b string) bool {
	if jsLikeExts[a] && jsLikeExts[b] {
		return true
	}
	return a == b
}
