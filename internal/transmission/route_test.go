package transmission

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

func testConfig(t *testing.T, endpoint, destination string) Config {
	t.Helper()
	data, err := json.Marshal(Config{RPCURL: endpoint, Rules: []Rule{
		{Name: "custom", Pattern: `(?i)custom`},
		{Name: "series", Pattern: `(?i)s01e02`, Destination: destination},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRouteThroughRPC(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "séries with spaces")
	if err := os.Mkdir(destination, 0755); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/transmission/rpc" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected RPC request: %s %s", r.Method, r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "raspbibi" || pass != "secret" {
			t.Error("missing RPC credentials")
		}
		var request struct {
			Method    string `json:"method"`
			Arguments struct {
				IDs      []string `json:"ids"`
				Location string   `json:"location"`
				Move     bool     `json:"move"`
			} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Method != "torrent-set-location" || len(request.Arguments.IDs) != 1 || request.Arguments.IDs[0] != testHash || request.Arguments.Location != destination || !request.Arguments.Move {
			t.Errorf("unexpected RPC payload: %+v", request)
		}
		if call == 1 {
			w.Header().Set(sessionHeader, "session-token")
			w.WriteHeader(http.StatusConflict)
			return
		}
		if r.Header.Get(sessionHeader) != "session-token" {
			t.Error("missing session token on retry")
		}
		io.WriteString(w, `{"result":"success","arguments":{}}`)
	}))
	defer server.Close()
	cfg := testConfig(t, server.URL+"/transmission/rpc", destination)
	var output bytes.Buffer
	err := Run(context.Background(), cfg, Torrent{Name: "Show.S01E02", Hash: testHash}, Client{Username: "raspbibi", Password: "secret"}, false, &output)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || !strings.Contains(output.String(), "Requested relocation") {
		t.Fatalf("calls = %d; output = %q", calls.Load(), output.String())
	}
}

func TestRouteDoesNotMutateForSkippedOrInvalidTorrents(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"result":"success"}`)
	}))
	defer server.Close()
	for _, tt := range []struct {
		name, torrentName, hash  string
		dryRun, sameDir, wantErr bool
	}{
		{"unknown", "Unknown", testHash, false, false, false},
		{"custom priority", "Custom.Show.S01E02", testHash, false, false, false},
		{"dry run", "Show.S01E02", testHash, true, false, false},
		{"same directory", "Show.S01E02", testHash, false, true, false},
		{"missing directory", "Show.S01E02", testHash, false, false, true},
		{"missing name", "", testHash, false, false, true},
		{"missing hash", "Show.S01E02", "", false, false, true},
		{"unsafe selector", "Show.S01E02", "all", false, false, true},
		{"nonhex hash", "Show.S01E02", strings.Repeat("z", 40), false, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "must-not-be-created")
			cfg := testConfig(t, server.URL, destination)
			torrent := Torrent{Name: tt.torrentName, Hash: tt.hash}
			if tt.sameDir {
				torrent.Dir = destination + "/"
			}
			err := Run(context.Background(), cfg, torrent, Client{}, tt.dryRun, io.Discard)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v; wantErr = %v", err, tt.wantErr)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("destination was created, stat error = %v", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("unexpected RPC calls: %d", calls.Load())
	}
}

func TestRouteRejectsFileDestinationAndCancellation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(destination, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, DefaultRPCURL, destination)
	torrent := Torrent{Name: "Show.S01E02", Hash: testHash}
	if err := Run(context.Background(), cfg, torrent, Client{}, false, io.Discard); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected directory error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, cfg, torrent, Client{}, false, io.Discard); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
