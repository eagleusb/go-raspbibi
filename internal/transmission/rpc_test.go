package transmission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustRPCClient(t *testing.T, endpoint, username, password string) Client {
	t.Helper()
	client, err := NewClient(endpoint, username, password)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeRPCReply(w io.Writer, tag int, body string) {
	fmt.Fprintf(w, `{"tag":%d,%s`, tag, strings.TrimPrefix(body, "{"))
}

func TestNewClient(t *testing.T) {
	mustRPCClient(t, "", "", "")
	for _, endpoint := range []string{
		"file:///tmp/rpc",
		"http://:9091/rpc",
		"http://user:secret@localhost/rpc",
		"http://localhost/rpc?password=secret",
		"http://localhost/rpc?",
		"http://localhost/rpc#fragment",
		"http://localhost/rpc#",
	} {
		if _, err := NewClient(endpoint, "", ""); err == nil {
			t.Errorf("accepted invalid endpoint %q", endpoint)
		}
	}
}

func TestRPCCredentialsRequireHTTPSOutsideLoopback(t *testing.T) {
	for _, endpoint := range []string{
		"http://192.0.2.1/rpc",
		"http://transmission.example/rpc",
		"http://localhost.example/rpc",
	} {
		t.Run(endpoint, func(t *testing.T) {
			client := mustRPCClient(t, endpoint, "user", "password")
			err := client.Move(t.Context(), testHash, "/destination")
			if err == nil || !strings.Contains(err.Error(), "HTTPS is required for authenticated non-loopback RPC") {
				t.Fatalf("move to %q returned %v, want HTTPS requirement", endpoint, err)
			}
		})
	}
}

func TestRPCFailures(t *testing.T) {
	for _, tt := range []struct {
		name, body, token, want string
		status, calls           int
		echoTag                 bool
	}{
		{"unauthorized", "", "", "HTTP error 401", 401, 1, false},
		{"forbidden", "", "", "HTTP error 403", 403, 1, false},
		{"server error", "", "", "HTTP error 500", 500, 1, false},
		{"missing challenge token", "", "", "no X-Transmission-Session-Id", 409, 1, false},
		{"repeated challenge", "", "token", "CSRF token invalid 2 times", 409, 2, false},
		{"RPC error", `{"result":"permission denied"}`, "", "permission denied", 200, 1, true},
		{"missing result", `{"arguments":{}}`, "", "does not indicate success", 200, 1, true},
		{"missing tag", `{"result":"success"}`, "", "does not have a tag", 200, 1, false},
		{"malformed JSON", `<html>failure</html>`, "", "invalid JSON", 200, 1, false},
		{"trailing response", `{"result":"success"} garbage`, "", "invalid JSON", 200, 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set(sessionHeader, tt.token)
				w.WriteHeader(tt.status)
				if tt.echoTag {
					var request struct {
						Tag int `json:"tag"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					writeRPCReply(w, request.Tag, tt.body)
				} else {
					io.WriteString(w, tt.body)
				}
			}))
			defer server.Close()
			client := mustRPCClient(t, server.URL, "", "")
			err := client.Move(t.Context(), testHash, "/destination")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
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
	client := mustRPCClient(t, server.URL, "user", "secret")
	err := client.Move(t.Context(), testHash, "/destination")
	if err == nil || !strings.Contains(err.Error(), "HTTP error 307") || calls.Load() != 0 {
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
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	client := mustRPCClient(t, server.URL, "", "")
	err := client.Move(ctx, testHash, "/destination")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected RPC deadline error, got %v", err)
	}
}
