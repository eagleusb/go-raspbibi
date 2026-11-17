package transmission

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPCFailures(t *testing.T) {
	for _, tt := range []struct {
		name, body, token, want string
		status, calls           int
	}{
		{"unauthorized", "", "", "HTTP 401", 401, 1},
		{"forbidden", "", "", "HTTP 403", 403, 1},
		{"server error", "", "", "HTTP 500", 500, 1},
		{"missing challenge token", "", "", "no X-Transmission-Session-Id", 409, 1},
		{"repeated challenge", "", "token", "HTTP 409", 409, 2},
		{"RPC error", `{"result":"permission denied"}`, "", "permission denied", 200, 1},
		{"missing result", `{}`, "", "RPC failed", 200, 1},
		{"malformed JSON", `<html>failure</html>`, "", "decode RPC response", 200, 1},
		{"trailing response", `{"result":"success"} garbage`, "", "exactly one JSON object", 200, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set(sessionHeader, tt.token)
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()
			err := (Client{URL: server.URL}).Move(context.Background(), testHash, "/destination")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v; want %q", err, tt.want)
			}
			if calls.Load() != int32(tt.calls) {
				t.Fatalf("calls = %d, want %d", calls.Load(), tt.calls)
			}
		})
	}
}

func TestRPCDoesNotFollowRedirect(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	err := (Client{URL: server.URL, Username: "user", Password: "secret"}).Move(context.Background(), testHash, "/destination")
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") || calls.Load() != 0 {
		t.Fatalf("error = %v, redirect target calls = %d", err, calls.Load())
	}
}

func TestRPCTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := (Client{URL: server.URL}).Move(ctx, testHash, "/destination")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected RPC deadline error, got %v", err)
	}
}
