package spec

import "testing"

func TestParseDirectRoutes(t *testing.T) {
	src := `
package router

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.GET("/status", controller.GetStatus)
	apiRouter.POST("/setup", controller.PostSetup)
	channelRoute := apiRouter.Group("/channel")
	channelRoute.GET("/test/:id", controller.TestChannel)
	channelRoute.DELETE("/:id", controller.DeleteChannel)
}
`
	got := ParseRouterFile(src)
	want := []string{
		"GET /api/status",
		"POST /api/setup",
		"GET /api/channel/test/:id",
		"DELETE /api/channel/:id",
	}
	assertRoutes(t, got, want)
}

func TestParsePermissionHelperAcrossLines(t *testing.T) {
	src := `
package router

func registerChannelRoutes(apiRouter *gin.RouterGroup) {
	channelRoute := apiRouter.Group("/channel")
	handlePermissionRoute(channelRoute,
		"GET",
		"/ops",
		authz.ChannelRead,
		controller.GetChannelOps,
	)
	handlePermissionRoute(channelRoute, "POST", "/priority/reorder", authz.ChannelWrite, controller.ReorderChannelPriority)
}
`
	got := ParseRouterFile(src)
	want := []string{
		"GET /api/channel/ops",
		"POST /api/channel/priority/reorder",
	}
	assertRoutes(t, got, want)
}

// The table-driven style is how the main channel and user CRUD are registered in
// modern New API. A parser that only understands group.METHOD("...") misses all
// of it, which is the single biggest source of wrong coverage numbers.
func TestParseTableDrivenRoutes(t *testing.T) {
	src := `
package router

func registerChannelRoutes(apiRouter *gin.RouterGroup) {
	channelRoute := apiRouter.Group("/channel")
	for _, route := range channelPermissionRoutes {
		handlePermissionRoute(channelRoute, route.method, route.path, route.permission, route.handler)
	}
}

var channelPermissionRoutes = []permissionRoute{
	{method: http.MethodGet, path: "/", permission: authz.ChannelRead, handler: controller.GetAllChannels},
	{method: http.MethodPost, path: "/", permission: authz.ChannelWrite, handler: controller.AddChannel},
	{method: http.MethodPut, path: "/", permission: authz.ChannelWrite, handler: controller.UpdateChannel},
	{method: http.MethodPost, path: "/:id/status", permission: authz.ChannelOperate, handler: controller.UpdateChannelStatus},
	{method: http.MethodGet, path: "/upstream_updates/detect", permission: authz.ChannelRead, handler: controller.Detect},
}
`
	got := ParseRouterFile(src)
	want := []string{
		"GET /api/channel/",
		"POST /api/channel/",
		"PUT /api/channel/",
		"POST /api/channel/:id/status",
		"GET /api/channel/upstream_updates/detect",
	}
	assertRoutes(t, got, want)
}

func TestParseSkipsComments(t *testing.T) {
	src := `
package router
func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	//apiRouter.GET("/midjourney", controller.GetMidjourney)
	apiRouter.GET("/notice", controller.GetNotice) // trailing comment
}
`
	got := ParseRouterFile(src)
	want := []string{"GET /api/notice"}
	assertRoutes(t, got, want)
}

// A wrong seed for the root router turns every path into /api/api/...
func TestParseDoesNotDoubleTheAPIPrefix(t *testing.T) {
	src := `
package router
func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.GET("/status", controller.GetStatus)
}
`
	got := ParseRouterFile(src)
	if len(got) != 1 {
		t.Fatalf("expected 1 route, got %d: %+v", len(got), got)
	}
	if got[0].Path != "/api/status" {
		t.Fatalf("expected /api/status, got %q", got[0].Path)
	}
}

// A file with several sibling /api groups must not attribute its table to a guess.
func TestParseTableAmbiguousGroupIsSkipped(t *testing.T) {
	src := `
package router
func register(apiRouter *gin.RouterGroup) {
	aRoute := apiRouter.Group("/alpha")
	bRoute := apiRouter.Group("/beta")
	_ = aRoute
	_ = bRoute
}
var table = []permissionRoute{
	{method: http.MethodGet, path: "/x", permission: p, handler: h},
}
`
	got := ParseRouterFile(src)
	if len(got) != 0 {
		t.Fatalf("expected no routes when the table target is ambiguous, got %+v", got)
	}
}

func TestGroupOf(t *testing.T) {
	cases := map[string]string{
		"/api/channel/":         "channel",
		"/api/channel/:id":      "channel",
		"/api/status":           "status",
		"/api":                  "root",
		"/api/token/batch/keys": "token",
	}
	for in, want := range cases {
		if got := GroupOf(in); got != want {
			t.Errorf("GroupOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiff(t *testing.T) {
	local := []Route{{Method: "GET", Path: "/api/a"}, {Method: "GET", Path: "/api/b"}}
	upstream := []Route{{Method: "GET", Path: "/api/b"}, {Method: "GET", Path: "/api/c"}}
	missing, stale := Diff(local, upstream)
	if len(missing) != 1 || missing[0].Path != "/api/c" {
		t.Fatalf("missing = %+v, want /api/c", missing)
	}
	if len(stale) != 1 || stale[0].Path != "/api/a" {
		t.Fatalf("stale = %+v, want /api/a", stale)
	}
}

func assertRoutes(t *testing.T, got []Route, want []string) {
	t.Helper()
	have := map[string]bool{}
	for _, r := range got {
		have[r.Method+" "+r.Path] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("missing route %q (parsed: %v)", w, keysOf(got))
		}
	}
	if len(got) != len(want) {
		t.Errorf("parsed %d routes, want %d: %v", len(got), len(want), keysOf(got))
	}
}

func keysOf(rs []Route) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}
