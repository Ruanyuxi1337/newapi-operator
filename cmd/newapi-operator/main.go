// Command newapi-operator is an MCP server that gives an AI agent full
// administrative control over a self-hosted New API LLM gateway.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ruanyuxi1337/newapi-operator/internal/client"
	"github.com/Ruanyuxi1337/newapi-operator/internal/config"
	"github.com/Ruanyuxi1337/newapi-operator/internal/spec"
	"github.com/Ruanyuxi1337/newapi-operator/internal/tools"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("newapi-operator %s (route table: New API %s, %d admin routes)\n",
			version, spec.UpstreamVersion, len(spec.Routes))
		return
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "newapi-operator:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// stdio owns stdout for the MCP protocol — all logging goes to stderr.
	if cfg.Transport == config.TransportStdio {
		logger.Info("starting newapi-operator",
			"version", version,
			"transport", string(cfg.Transport),
			"base_url", cfg.BaseURL,
			"read_only", cfg.ReadOnly,
			"route_table", spec.UpstreamVersion,
			"routes", len(spec.Routes),
		)
	}

	apiClient := client.NewWithOptions(client.Options{
		BaseURL:  cfg.BaseURL,
		Token:    cfg.AccessToken,
		UserID:   cfg.UserID,
		HTTP:     &http.Client{Timeout: cfg.Timeout},
		ReadOnly: cfg.ReadOnly,
	})

	registry := tools.NewRegistry(apiClient, cfg.QuotaPerUnit)

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "newapi-operator",
			Version: version,
		},
		&mcp.ServerOptions{
			Instructions: tools.Instructions(version, spec.UpstreamVersion),
			PageSize:     100,
		},
	)
	tools.RegisterAll(server, registry)
	logger.Info("tools registered", "count", len(registry.Tools()))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch cfg.Transport {
	case config.TransportStdio:
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stdio transport: %w", err)
		}
		return nil
	case config.TransportHTTP:
		return runHTTP(ctx, cfg, server, logger)
	default:
		return fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
}

func runHTTP(ctx context.Context, cfg *config.Config, server *mcp.Server, logger *slog.Logger) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", bearerAuth(cfg.HTTPToken, handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":%q,"route_table":%q}`, version, spec.UpstreamVersion)
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Info("http transport listening", "addr", cfg.HTTPAddr, "path", "/mcp")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// bearerAuth guards the HTTP transport. The MCP endpoint is a full admin
// control plane, so an unauthenticated listener is never acceptable.
func bearerAuth(token string, next http.Handler) http.Handler {
	const prefix = "Bearer "
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got := strings.TrimSpace(header[len(prefix):])
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(cfg.LogFormat, "json") {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}
