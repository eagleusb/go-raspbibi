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
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

func testRules(destination string) []routeRule {
	return []routeRule{
		{name: "custom", pattern: regexp.MustCompile(`(?i)custom`)},
		{name: "series", pattern: regexp.MustCompile(`(?i)s01e02`), destination: destination},
	}
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
			Tag       int    `json:"tag"`
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
		writeRPCReply(w, request.Tag, `{"result":"success","arguments":{}}`)
	}))
	defer server.Close()
	client := mustRPCClient(t, server.URL+"/transmission/rpc", "raspbibi", "secret")
	var output bytes.Buffer
	err := run(context.Background(), testRules(destination), Torrent{Name: "Show.S01E02", Hash: testHash}, client, false, &output)
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
		{"same directory dry run", "Show.S01E02", testHash, true, true, false},
		{"missing directory", "Show.S01E02", testHash, false, false, true},
		{"missing name", "", testHash, false, false, true},
		{"missing hash", "Show.S01E02", "", false, false, true},
		{"unsafe selector", "Show.S01E02", "all", false, false, true},
		{"nonhex hash", "Show.S01E02", strings.Repeat("z", 40), false, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "must-not-be-created")
			torrent := Torrent{Name: tt.torrentName, Hash: tt.hash}
			if tt.sameDir {
				torrent.Dir = destination + "/"
			}
			client := mustRPCClient(t, server.URL, "", "")
			err := run(context.Background(), testRules(destination), torrent, client, tt.dryRun, io.Discard)
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

func TestRouteUsesFinalDownloadDirBeforeSkipping(t *testing.T) {
	destination := t.TempDir()
	for _, tt := range []struct {
		name, finalDir string
		moves          int32
		refreshSession bool
	}{
		{"already routed", destination + "/", 0, false},
		{"current directory is incomplete", t.TempDir(), 1, false},
		{"session refresh before move", t.TempDir(), 2, true},
		{"missing download directory", "", 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gets, moves atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				token := r.Header.Get(sessionHeader)
				if token == "" {
					w.Header().Set(sessionHeader, "initial")
					w.WriteHeader(http.StatusConflict)
					return
				}
				var request struct {
					Method    string `json:"method"`
					Tag       int    `json:"tag"`
					Arguments struct {
						IDs      []string `json:"ids"`
						Location string   `json:"location"`
						Move     bool     `json:"move"`
						Fields   []string `json:"fields"`
					} `json:"arguments"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if ids := request.Arguments.IDs; len(ids) != 1 || ids[0] != testHash {
					t.Errorf("unexpected torrent selector: %v", ids)
				}
				switch request.Method {
				case "torrent-get":
					gets.Add(1)
					if fields := request.Arguments.Fields; len(fields) != 1 || fields[0] != "downloadDir" {
						t.Errorf("unexpected fields: %v", fields)
					}
					directory, err := json.Marshal(tt.finalDir)
					if err != nil {
						t.Error(err)
						return
					}
					writeRPCReply(w, request.Tag, `{"result":"success","arguments":{"torrents":[{"downloadDir":`+string(directory)+`}]}}`)
				case "torrent-set-location":
					moves.Add(1)
					if gets.Load() != 1 || request.Arguments.Location != destination || !request.Arguments.Move {
						t.Errorf("unexpected relocation: %+v", request.Arguments)
					}
					if tt.refreshSession {
						if token == "initial" {
							w.Header().Set(sessionHeader, "refreshed")
							w.WriteHeader(http.StatusConflict)
							return
						}
						if token != "refreshed" {
							t.Errorf("move used token %q, want refreshed", token)
						}
					}
					writeRPCReply(w, request.Tag, `{"result":"success"}`)
				default:
					t.Errorf("unexpected method: %q", request.Method)
				}
			}))
			defer server.Close()
			torrent := Torrent{Name: "Show.S01E02", Hash: testHash, Dir: destination}
			client := mustRPCClient(t, server.URL, "", "")
			err := run(context.Background(), testRules(destination), torrent, client, false, io.Discard)
			if (err != nil) != (tt.finalDir == "") || gets.Load() != 1 || moves.Load() != tt.moves {
				t.Fatalf("error = %v, lookups = %d, moves = %d (want %d)", err, gets.Load(), moves.Load(), tt.moves)
			}
		})
	}
}

func TestRoutePreviewsWithRemoteHTTPCredentials(t *testing.T) {
	client := mustRPCClient(t, "http://transmission.example/rpc", "user", "password")
	destination := filepath.Join(t.TempDir(), "series")
	var output bytes.Buffer
	torrent := Torrent{Name: "Show.S01E02", Hash: testHash}
	if err := run(t.Context(), testRules(destination), torrent, client, true, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), destination) {
		t.Errorf("preview = %q, want destination %q", output.String(), destination)
	}
}

func TestRouteRejectsFileDestinationAndCancellation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(destination, nil, 0600); err != nil {
		t.Fatal(err)
	}
	rules := testRules(destination)
	torrent := Torrent{Name: "Show.S01E02", Hash: testHash}
	if err := run(context.Background(), rules, torrent, Client{}, false, io.Discard); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected directory error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, rules, torrent, Client{}, false, io.Discard); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
