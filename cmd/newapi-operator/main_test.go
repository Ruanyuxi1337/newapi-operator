package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := bearerAuth("s3cret-token", next)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer s3cret-token", http.StatusNoContent},
		{"valid lowercase scheme", "bearer s3cret-token", http.StatusNoContent},
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "s3cret-token", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"bare scheme", "Bearer ", http.StatusUnauthorized},
		{"prefix of token", "Bearer s3cret", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
