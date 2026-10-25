// Command routesync regenerates the embedded New API route table from upstream
// router source, and detects drift between what this server knows and what a
// given New API release actually exposes.
//
// Usage:
//
//	routesync --tag v1.0.0-rc.47              # regenerate routes_gen.go
//	routesync --tag v1.0.0-rc.47 --check      # exit 1 if the table would change
//	routesync --tag latest --check            # drift check against newest release
//	routesync --tag v1.0.0-rc.47 --report     # human-readable coverage report
//
// The parser handles the three registration styles New API mixes in router/*.go;
// see internal/spec/parse.go.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Ruanyuxi1337/newapi-operator/internal/spec"
)

const defaultRepo = "QuantumNous/new-api"

type options struct {
	repo          string
	tag           string
	out           string
	baseline      string
	check         bool
	report        bool
	jsonOut       bool
	writeBaseline bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "routesync:", err)
		os.Exit(1)
	}
}

func run() error {
	var o options
	flag.StringVar(&o.repo, "repo", defaultRepo, "upstream GitHub repo owner/name")
	flag.StringVar(&o.tag, "tag", "latest", "New API git tag, or 'latest' to resolve the newest release")
	flag.StringVar(&o.out, "out", "internal/spec/routes_gen.go", "generated Go file path")
	flag.StringVar(&o.baseline, "baseline", "internal/spec/routes.baseline.json", "baseline JSON to diff against")
	flag.BoolVar(&o.check, "check", false, "exit 1 when the generated table differs from the baseline")
	flag.BoolVar(&o.report, "report", false, "print a per-group coverage report")
	flag.BoolVar(&o.jsonOut, "json", false, "print the generated table as JSON to stdout")
	flag.BoolVar(&o.writeBaseline, "write-baseline", false, "overwrite the baseline JSON with the freshly generated table")
	flag.Parse()

	tag := o.tag
	if tag == "latest" {
		resolved, err := latestRelease(o.repo)
		if err != nil {
			return fmt.Errorf("resolve latest release: %w", err)
		}
		tag = resolved
	}

	routes, err := fetchRoutes(o.repo, tag)
	if err != nil {
		return err
	}
	if err := spec.Validate(routes); err != nil {
		return fmt.Errorf("validate routes: %w", err)
	}
	spec.SortRoutes(routes)

	if o.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(routes); err != nil {
			return err
		}
	}

	if o.report {
		printReport(tag, routes)
	}

	if o.out != "" {
		if err := writeGoFile(o.out, tag, routes); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d routes, upstream %s)\n", o.out, len(routes), tag)
	}

	if o.writeBaseline {
		if err := writeBaselineJSON(o.baseline, routes); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "baseline written: %s (%d routes)\n", o.baseline, len(routes))
	}

	if o.check {
		changed, detail, err := compareBaseline(o.baseline, routes)
		if err != nil {
			return err
		}
		if changed {
			fmt.Fprintln(os.Stderr, "drift detected against baseline:")
			fmt.Fprintln(os.Stderr, detail)
			return fmt.Errorf("route table is out of date for %s; run routesync --tag %s and review the diff", tag, tag)
		}
		fmt.Fprintf(os.Stderr, "no drift: %d routes match the baseline\n", len(routes))
	}
	return nil
}

// ---- upstream fetch -------------------------------------------------------

func httpClient() *http.Client { return &http.Client{Timeout: 60 * time.Second} }

func ghRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "newapi-operator-routesync")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return httpClient().Do(req)
}

func latestRelease(repo string) (string, error) {
	resp, err := ghRequest("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("release has no tag_name")
	}
	return rel.TagName, nil
}

func listRouterFiles(repo, tag string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/router?ref=%s", repo, tag)
	resp, err := ghRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("list router dir: github API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		// Router source only; skip tests, which register nothing real.
		if !strings.HasSuffix(e.Name, ".go") || strings.HasSuffix(e.Name, "_test.go") {
			continue
		}
		files = append(files, e.Name)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no router Go files found at %s@%s", repo, tag)
	}
	return files, nil
}

func fetchRoutes(repo, tag string) ([]spec.Route, error) {
	files, err := listRouterFiles(repo, tag)
	if err != nil {
		return nil, err
	}
	seen := map[string]spec.Route{}
	for _, name := range files {
		raw := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/router/%s", repo, tag, name)
		resp, err := httpClient().Get(raw)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", name, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: HTTP %d", name, resp.StatusCode)
		}
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", name, readErr)
		}
		for _, r := range spec.ParseRouterFile(string(body)) {
			r.Source = name
			seen[r.Key()] = r
		}
	}
	out := make([]spec.Route, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	spec.SortRoutes(out)
	return out, nil
}

// ---- output ---------------------------------------------------------------

func writeGoFile(path, tag string, routes []spec.Route) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("// Code generated by cmd/routesync. DO NOT EDIT.\n\n")
	b.WriteString("package spec\n\n")
	fmt.Fprintf(&b, "// UpstreamVersion is the New API git tag this table was extracted from.\nconst UpstreamVersion = %q\n\n", tag)
	b.WriteString("// Routes is every admin (method, path) pair registered by that release.\nvar Routes = []Route{\n")
	for _, r := range routes {
		fmt.Fprintf(&b, "\t{Method: %q, Path: %q, Group: %q, Source: %q},\n", r.Method, r.Path, r.Group, r.Source)
	}
	b.WriteString("}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeBaselineJSON(path string, routes []spec.Route) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(routes, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func compareBaseline(path string, routes []spec.Route) (bool, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// First run: treat as drift so the baseline gets committed.
			return true, "baseline file does not exist yet", nil
		}
		return false, "", err
	}
	var baseline []spec.Route
	if err := json.Unmarshal(raw, &baseline); err != nil {
		return false, "", fmt.Errorf("parse baseline %s: %w", path, err)
	}
	missing, stale := spec.Diff(baseline, routes)
	if len(missing) == 0 && len(stale) == 0 {
		return false, "", nil
	}
	var b strings.Builder
	for _, r := range missing {
		fmt.Fprintf(&b, "  + %-6s %s\n", r.Method, r.Path)
	}
	for _, r := range stale {
		fmt.Fprintf(&b, "  - %-6s %s\n", r.Method, r.Path)
	}
	return true, b.String(), nil
}

func printReport(tag string, routes []spec.Route) {
	byGroup := map[string]int{}
	for _, r := range routes {
		byGroup[r.Group]++
	}
	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return byGroup[groups[i]] > byGroup[groups[j]] })

	fmt.Printf("New API %s — %d admin routes\n\n", tag, len(routes))
	fmt.Printf("%-24s %6s\n", "GROUP", "ROUTES")
	fmt.Printf("%-24s %6s\n", "-----", "------")
	for _, g := range groups {
		fmt.Printf("%-24s %6d\n", g, byGroup[g])
	}
}
