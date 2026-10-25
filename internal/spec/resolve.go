package spec

import "strings"

// pathIndex maps a canonical path to every literal spelling the table contains.
var pathIndex = func() map[string]string {
	m := make(map[string]string, len(Routes)*2)
	for _, r := range Routes {
		key := r.Method + " " + CanonicalPath(r.Path)
		if _, exists := m[key]; !exists {
			m[key] = r.Path
		}
	}
	return m
}()

// Resolve maps a method+path onto the spelling the running release actually
// registered, tolerating a missing or extra trailing slash.
//
// Gin treats "/api/log" and "/api/log/" as different routes. New API is not
// consistent about which one it registers, and a mismatch is expensive: a GET
// survives the 301 redirect, but a POST or PUT turns into a GET and silently
// loses its body. Resolving against the table turns that class of bug into a
// non-event.
//
// ok is false when the pair is not in the table at all; the caller should then
// decide whether to send it anyway (api_request) or fail.
func Resolve(method, path string) (resolved string, ok bool) {
	key := strings.ToUpper(method) + " " + CanonicalPath(path)
	if literal, found := pathIndex[key]; found {
		return literal, true
	}
	return path, false
}

// Has reports whether the pair is in the table, ignoring a trailing-slash difference.
func Has(method, path string) bool {
	_, ok := Resolve(method, path)
	return ok
}

// Paths returns every registered path, for diagnostics.
func Paths() []string {
	out := make([]string, 0, len(Routes))
	for _, r := range Routes {
		out = append(out, r.Path)
	}
	return out
}

// Match reports whether a concrete request path matches a registered route,
// treating ":param" segments as single-segment wildcards.
//
// Has answers "is this exact spelling in the table"; Match answers "would the
// gateway route this request". Tools call concrete paths such as
// /api/channel/7, so Match is the right question to ask about them.
func Match(method, path string) (Route, bool) {
	method = strings.ToUpper(method)
	want := strings.Split(strings.Trim(CanonicalPath(path), "/"), "/")
	for _, r := range Routes {
		if r.Method != method {
			continue
		}
		have := strings.Split(strings.Trim(CanonicalPath(r.Path), "/"), "/")
		if len(have) != len(want) {
			continue
		}
		ok := true
		for i := range have {
			if strings.HasPrefix(have[i], ":") {
				if want[i] == "" {
					ok = false
					break
				}
				continue
			}
			if have[i] != want[i] {
				ok = false
				break
			}
		}
		if ok {
			return r, true
		}
	}
	return Route{}, false
}
