package projectmap

// HTTP routes read from source literals: deterministic, language-aware and bounded.

import (
	"regexp"
	"strings"
)

const maxRoutesPerFile = 50

// Route is one HTTP endpoint declared in the code, with the file that declares it.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	File   string `json:"file"`
}

var (
	reRouteJs       = regexp.MustCompile(`(?m)\b(?:app|router|server|routes?|fastify|route)\s*\.\s*(get|post|put|patch|delete|head|options|all)\s*\(\s*['"]([^'"]+)['"]`)
	reRouteGoVerb   = regexp.MustCompile(`(?m)\b(?:r|e|g|router|mux|group)\s*\.\s*(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|Any|Get|Post|Put|Patch|Delete|Head|Options)\s*\(\s*"([^"]+)"`)
	reRouteGoHandle = regexp.MustCompile(`(?m)\b(?:HandleFunc|Handle)\s*\(\s*"([^"]+)"`)
	reRoutePy       = regexp.MustCompile(`(?m)@\w+(?:\.\w+)*\.(get|post|put|patch|delete|route)\(\s*['"]([^'"]+)['"]`)
	rePyMethods     = regexp.MustCompile(`methods\s*=\s*\[([^\]]*)\]`)
	reRouteDjango   = regexp.MustCompile(`(?m)^\s*(?:path|re_path)\s*\(\s*r?['"]([^'"]+)['"]`)
	reRouteCs       = regexp.MustCompile(`(?m)\[Http(Get|Post|Put|Patch|Delete|Head|Options)(?:Attribute)?(?:\(\s*"([^"]*)"\s*\))?\]`)
	reRouteCsRoute  = regexp.MustCompile(`(?m)\[Route\(\s*"([^"]+)"`)
	reRouteJava     = regexp.MustCompile(`(?m)@(Get|Post|Put|Patch|Delete|Request)Mapping\(\s*(?:value\s*=\s*)?(?:path\s*=\s*)?["']([^"']+)["']`)
	reRouteRust     = regexp.MustCompile(`(?m)#\[(get|post|put|patch|delete)\(\s*"([^"]+)"`)
	reRouteAxum     = regexp.MustCompile(`(?m)\.route\(\s*"([^"]+)"\s*,`)
	reRouteRails    = regexp.MustCompile(`(?m)^\s*(get|post|put|patch|delete)\s+['"]([^'"]+)['"]`)
	reRoutePhp      = regexp.MustCompile(`(?m)Route::(get|post|put|patch|delete|any)\s*\(\s*['"]([^'"]+)['"]`)
)

var goPatternMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true,
}

// extractRoutes reads the route literals a file declares. base is the file name (urls.py,
// routes.rb select framework conventions); relFile is recorded on each route.
func extractRoutes(text, lang, base, relFile string) []Route {
	var out []Route
	seen := map[string]bool{}
	switch lang {
	case "ts", "js":
		for _, mm := range reRouteJs.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, mm[1], mm[2], relFile)
		}
	case "go":
		for _, mm := range reRouteGoVerb.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, strings.ToUpper(mm[1]), mm[2], relFile)
		}
		for _, mm := range reRouteGoHandle.FindAllStringSubmatch(text, -1) {
			method, p := splitGoPattern(mm[1])
			addRoute(&out, seen, method, p, relFile)
		}
	case "py":
		extractPythonRoutes(text, relFile, seen, &out)
		if base == "urls.py" || base == "routes.py" {
			for _, mm := range reRouteDjango.FindAllStringSubmatch(text, -1) {
				addRoute(&out, seen, "", mm[1], relFile)
			}
		}
	case "cs":
		for _, mm := range reRouteCs.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, mm[1], mm[2], relFile)
		}
		for _, mm := range reRouteCsRoute.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, "", mm[1], relFile)
		}
	case "java", "kotlin":
		for _, mm := range reRouteJava.FindAllStringSubmatch(text, -1) {
			method := strings.ToUpper(mm[1])
			if method == "REQUEST" {
				method = ""
			}
			addRoute(&out, seen, method, mm[2], relFile)
		}
	case "rs":
		for _, mm := range reRouteRust.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, mm[1], mm[2], relFile)
		}
		for _, mm := range reRouteAxum.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, "", mm[1], relFile)
		}
	case "rb":
		if base == "routes.rb" {
			for _, mm := range reRouteRails.FindAllStringSubmatch(text, -1) {
				addRoute(&out, seen, mm[1], mm[2], relFile)
			}
		}
	case "php":
		for _, mm := range reRoutePhp.FindAllStringSubmatch(text, -1) {
			addRoute(&out, seen, mm[1], mm[2], relFile)
		}
	}
	return out
}

// extractPythonRoutes handles Flask/FastAPI decorators; a flask route() reads its methods list.
func extractPythonRoutes(text, relFile string, seen map[string]bool, out *[]Route) {
	for _, idx := range reRoutePy.FindAllStringSubmatchIndex(text, -1) {
		method := strings.ToUpper(text[idx[2]:idx[3]])
		p := text[idx[4]:idx[5]]
		methods := []string{}
		if method == "ROUTE" {
			tail := text[idx[1]:min(idx[1]+240, len(text))]
			if mm := rePyMethods.FindStringSubmatch(tail); len(mm) > 1 {
				for _, m := range strings.Split(mm[1], ",") {
					if v := strings.ToUpper(strings.Trim(strings.TrimSpace(m), `"'`)); v != "" {
						methods = append(methods, v)
					}
				}
			}
		}
		if len(methods) == 0 {
			addRoute(out, seen, method, p, relFile)
			continue
		}
		for _, m := range methods {
			addRoute(out, seen, m, p, relFile)
		}
	}
}

// splitGoPattern splits a Go 1.22 "GET /users/{id}" handler pattern into method and path.
func splitGoPattern(p string) (string, string) {
	fields := strings.SplitN(strings.TrimSpace(p), " ", 2)
	if len(fields) == 2 && goPatternMethods[strings.ToUpper(fields[0])] {
		return strings.ToUpper(fields[0]), fields[1]
	}
	return "", p
}

func addRoute(list *[]Route, seen map[string]bool, method, p, file string) {
	if len(*list) >= maxRoutesPerFile {
		return
	}
	p = strings.TrimSpace(p)
	p = strings.TrimSuffix(strings.TrimPrefix(p, "^"), "$")
	if p == "" || strings.ContainsAny(p, "\n\r") || strings.Contains(p, "[controller]") || strings.Contains(p, "[action]") {
		return
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" || method == "ANY" || method == "ALL" {
		method = "*"
	}
	key := method + " " + p
	if seen[key] {
		return
	}
	seen[key] = true
	*list = append(*list, Route{Method: method, Path: p, File: file})
}
