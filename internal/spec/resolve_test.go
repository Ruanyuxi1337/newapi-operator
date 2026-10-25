package spec

import "testing"

// New API is inconsistent about the trailing slash: it registers
// "/api/user/" but "/api/channel/" too, while other families use no slash at
// all. A mismatch is not cosmetic — a POST that hits a 301 arrives as a GET and
// loses its body. Resolve exists to make that impossible.
func TestResolveToleratesTrailingSlash(t *testing.T) {
	if !Has("GET", "/api/user/") {
		t.Fatal("table should contain /api/user/")
	}
	for _, in := range []string{"/api/user", "/api/user/"} {
		got, ok := Resolve("GET", in)
		if !ok {
			t.Fatalf("Resolve(%q) reported unknown", in)
		}
		if got != "/api/user/" {
			t.Errorf("Resolve(%q) = %q, want /api/user/", in, got)
		}
	}
}

func TestResolveReturnsLiteralSpelling(t *testing.T) {
	for _, r := range Routes {
		got, ok := Resolve(r.Method, r.Path)
		if !ok {
			t.Fatalf("Resolve(%s %s) reported unknown", r.Method, r.Path)
		}
		if got != r.Path {
			t.Errorf("Resolve(%s) = %q, want the literal %q", r.Path, got, r.Path)
		}
	}
}

func TestResolveUnknownPath(t *testing.T) {
	if _, ok := Resolve("GET", "/api/definitely-not-a-route"); ok {
		t.Error("unknown path must not resolve")
	}
	// Wrong method for a known path is also unknown.
	if _, ok := Resolve("DELETE", "/api/status"); ok {
		t.Error("a path registered for another method must not resolve")
	}
}

func TestResolveIsMethodAware(t *testing.T) {
	if !Has("POST", "/api/channel/") {
		t.Fatal("POST /api/channel/ should exist")
	}
	if Has("PATCH", "/api/channel/") {
		t.Error("PATCH /api/channel/ should not exist")
	}
}
