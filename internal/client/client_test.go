package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc, readOnly bool) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewWithOptions(Options{
		BaseURL:  srv.URL,
		Token:    "test-token",
		UserID:   "1",
		HTTP:     &http.Client{Timeout: 5 * time.Second},
		ReadOnly: readOnly,
	})
}

// New API reports a rejection as HTTP 200 with success:false. A client that
// trusts the status code alone treats every failure as a success.
func TestDoTreatsSuccessFalseAsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"message":"channel not found"}`))
	}, false)

	_, err := c.Do(context.Background(), "GET", "/api/channel/1", nil, nil)
	if err == nil {
		t.Fatal("expected an error for success:false, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !strings.Contains(apiErr.Message, "channel not found") {
		t.Errorf("error message lost the upstream text: %q", apiErr.Message)
	}
}

func TestDoSendsAuthAndUserHeaders(t *testing.T) {
	var gotAuth, gotUser string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUser = r.Header.Get("New-Api-User")
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}, false)

	if _, err := c.Do(context.Background(), "GET", "/api/channel/", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotUser != "1" {
		t.Errorf("New-Api-User = %q", gotUser)
	}
}

func TestDoMapsStatusCodes(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauth},
		{http.StatusForbidden, ErrUnauth},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusTooManyRequests, ErrRateLimit},
		{http.StatusBadGateway, ErrUpstream},
		{http.StatusBadRequest, ErrBadRequest},
	}
	for _, tc := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`upstream said no`))
		}, false)
		_, err := c.Do(context.Background(), "GET", "/api/status", nil, nil)
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: got %v, want %v", tc.status, err, tc.want)
		}
	}
}

func TestReadOnlyBlocksMutations(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"success":true}`))
	}, true)

	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if _, err := c.Do(context.Background(), m, "/api/channel/", nil, nil); !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s should be blocked, got %v", m, err)
		}
	}
	if called {
		t.Error("read-only mode still reached the upstream")
	}
	// Reads must still work.
	if _, err := c.Do(context.Background(), "GET", "/api/status", nil, nil); err != nil {
		t.Errorf("GET should be allowed in read-only mode: %v", err)
	}
}

func TestRefusesNonAdminPaths(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be reached for a non-/api path")
	}, false)
	for _, path := range []string{
		"/v1/chat/completions",
		"/api/../v1/chat/completions",
		"/api/%2e%2e/v1/chat/completions",
		"/api//v1/chat/completions",
		"/api/channel/1\r\nX-Injected: 1",
	} {
		if _, err := c.Do(context.Background(), "GET", path, nil, nil); err == nil {
			t.Errorf("%q: expected a path guard error", path)
		}
	}
}

func TestBodyIsEncodedAndContentTypeSet(t *testing.T) {
	var body map[string]any
	var ctype string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":3}}`))
	}, false)

	_, err := c.Do(context.Background(), "POST", "/api/channel/", nil, map[string]any{"mode": "single"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctype != "application/json" {
		t.Errorf("Content-Type = %q", ctype)
	}
	if body["mode"] != "single" {
		t.Errorf("body = %+v", body)
	}
}

func TestDecodeUnwrapsEnvelope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"total":7}}`))
	}, false)
	resp, err := c.Do(context.Background(), "GET", "/api/channel/", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Total int `json:"total"`
	}
	if err := resp.Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 7 {
		t.Errorf("Total = %d, want 7", out.Total)
	}
}
