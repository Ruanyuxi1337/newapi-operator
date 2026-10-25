// Package spec — route table integrity.
//
// These tests guard the contract with the running gateway: the committed table
// must match the generated one, and the baseline must match both. A failure here
// means someone edited a generated file by hand, or upstream added routes that
// were never adopted.
package spec

import (
	"encoding/json"
	"os"
	"testing"
)

// TestGeneratedTableMatchesBaseline fails when Routes (compiled from
// routes_gen.go) disagrees with routes.baseline.json. `routesync --check` does
// the same thing against freshly downloaded upstream source; this version needs
// no network and therefore runs on every commit.
func TestGeneratedTableMatchesBaseline(t *testing.T) {
	raw, err := os.ReadFile("routes.baseline.json")
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	var baseline []Route
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("parse baseline: %v", err)
	}
	if len(baseline) == 0 {
		t.Fatal("baseline is empty")
	}

	missing, stale := Diff(baseline, Routes)
	for _, r := range missing {
		t.Errorf("route present upstream but absent from the baseline: %s %s", r.Method, r.Path)
	}
	for _, r := range stale {
		t.Errorf("baseline lists a route the table does not have: %s %s", r.Method, r.Path)
	}
	if len(missing) == 0 && len(stale) == 0 && len(baseline) != len(Routes) {
		t.Errorf("baseline has %d routes, table has %d", len(baseline), len(Routes))
	}
}

func TestRoutesAreWellFormed(t *testing.T) {
	if err := Validate(Routes); err != nil {
		t.Fatalf("route table invalid: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range Routes {
		k := r.Method + " " + r.Path
		if seen[k] {
			t.Errorf("duplicate route %q", k)
		}
		seen[k] = true
		if r.Group == "" {
			t.Errorf("route %q has no group", k)
		}
	}
}

// The families the curated tools depend on must survive any regeneration.
func TestCriticalRoutesSurviveSync(t *testing.T) {
	required := []string{
		"GET /api/channel/",
		"POST /api/channel/",
		"PUT /api/channel/",
		"DELETE /api/channel/:id",
		"POST /api/channel/:id/status",
		"POST /api/channel/status/batch",
		"GET /api/channel/ops",
		"POST /api/channel/priority/reorder",
		"GET /api/channel/:id/codex/usage",
		"POST /api/channel/upstream_updates/detect",

		"GET /api/token/",
		"POST /api/token/",
		"PUT /api/token/",
		"DELETE /api/token/:id",
		"POST /api/token/:id/key",
		"POST /api/token/batch/keys",

		"GET /api/user",
		"POST /api/user/manage",
		"GET /api/option",
		"GET /api/option/model_pricing",
		"PATCH /api/option/model_pricing",
		"GET /api/log",
	}
	have := Index(Routes)
	for _, want := range required {
		found := false
		for k := range have {
			if CanonicalKey(k) == CanonicalKey(want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("critical route disappeared from the table: %s", want)
		}
	}
}

// CanonicalKey normalises a "METHOD /path" string for comparison.
func CanonicalKey(k string) string {
	for i := 0; i < len(k); i++ {
		if k[i] == ' ' {
			return k[:i] + " " + CanonicalPath(k[i+1:])
		}
	}
	return k
}
