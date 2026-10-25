package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// ParseRouterFile extracts every admin route registered in one New API router
// Go source file.
//
// Three registration styles coexist in New API and a regex for only one of them
// produces a wildly wrong table:
//
//	channelRoute.GET("/test/:id", handler)                       // direct
//	handlePermissionRoute(channelRoute, "GET", "/ops", perm, h)   // helper, often multi-line
//	var channelPermissionRoutes = []permissionRoute{             // table-driven, invisible
//	    {method: http.MethodGet, path: "/priority/order", ...},  //   to the two above
//	}
//
// Returns the routes and the parser's best guess at the file's primary /api group.
func ParseRouterFile(source string) []Route {
	src := stripComments(source)

	bases := map[string]string{
		"router":    "",
		"engine":    "",
		"r":         "",
		"Router":    "",
		"apiRouter": "/api",
	}

	groupRe := regexp.MustCompile(`(\w+)\s*:?=\s*(\w+)\.Group\(\s*"([^"]*)"`)
	for _, m := range groupRe.FindAllStringSubmatch(src, -1) {
		name, parent, fragment := m[1], m[2], m[3]
		base, ok := bases[parent]
		if !ok {
			continue
		}
		bases[name] = normaliseJoin(base + fragment)
	}

	var out []Route
	add := func(groupVar, method, path string) {
		base, ok := bases[groupVar]
		if !ok {
			return
		}
		full := normaliseJoin(base + "/" + strings.TrimPrefix(path, "/"))
		if !strings.HasPrefix(full, "/api") {
			return
		}
		out = append(out, Route{
			Method: strings.ToUpper(method),
			Path:   full,
			Group:  GroupOf(full),
		})
	}

	directRe := regexp.MustCompile(`(\w+)\.(GET|POST|PUT|DELETE|PATCH)\(\s*"([^"]*)"`)
	for _, m := range directRe.FindAllStringSubmatch(src, -1) {
		add(m[1], m[2], m[3])
	}

	helperRe := regexp.MustCompile(`handlePermissionRoute\(\s*(\w+)\s*,\s*"(GET|POST|PUT|DELETE|PATCH)"\s*,\s*"([^"]*)"`)
	for _, m := range helperRe.FindAllStringSubmatch(src, -1) {
		add(m[1], m[2], m[3])
	}

	// Table-driven registration: attribute the literal table to the file's only
	// non-root /api group, which is how channel-router.go and friends are built.
	tableRe := regexp.MustCompile(`\{method:\s*http\.Method(Get|Post|Put|Delete|Patch),\s*path:\s*"([^"]*)"`)
	if matches := tableRe.FindAllStringSubmatch(src, -1); len(matches) > 0 {
		if target := soleAPIGroup(bases); target != "" {
			for _, m := range matches {
				add(target, strings.ToUpper(m[1]), m[2])
			}
		}
	}

	return out
}

// soleAPIGroup returns the single group variable that lives directly under /api
// with a non-empty suffix, or "" when the file has zero or several candidates.
func soleAPIGroup(bases map[string]string) string {
	candidate := ""
	for name, base := range bases {
		if name == "apiRouter" {
			continue
		}
		if !strings.HasPrefix(base, "/api/") {
			continue
		}
		// Count depth: only the top-level group is a valid table target.
		if strings.Count(base, "/") != 2 {
			continue
		}
		if candidate != "" && candidate != name {
			return ""
		}
		candidate = name
	}
	return candidate
}

// normaliseJoin collapses duplicate slashes and strips a trailing slash so that
// "/api/channel/" and "/api/channel" compare equal.
func normaliseJoin(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p == "" {
		return "/"
	}
	return p
}

// stripComments removes // line comments that are not inside a string literal.
func stripComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	inRune := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				b.WriteByte(src[i])
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if inRune {
			b.WriteByte(c)
			if c == '\'' {
				inRune = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			b.WriteByte(c)
		case '\'':
			inRune = true
			b.WriteByte(c)
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
				if i < len(src) {
					b.WriteByte('\n')
				}
				continue
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Validate rejects obviously malformed routes before they reach the table.
func Validate(routes []Route) error {
	for _, r := range routes {
		if r.Method == "" || r.Path == "" {
			return fmt.Errorf("route with empty method or path: %+v", r)
		}
		if !strings.HasPrefix(r.Path, "/api") {
			return fmt.Errorf("route %s %s is outside /api", r.Method, r.Path)
		}
	}
	return nil
}
