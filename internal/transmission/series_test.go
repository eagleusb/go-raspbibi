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

func TestSeriesRouteThroughRPC(t *testing.T) {
	for _, tt := range []struct {
		name, root, title, season string
	}{
		{"Orange.Is.the.New.Black.S01.MULTi.1080p.NF.WEB.DDP.AV1-BTT", "series", "Orange.Is.The.New.Black", "s01"},
		{"MobLand.S02E09.MULTi.4KLight.DV.HDR10+.WEBRip.DDP5.1.Atmos.x265-[BATGirl].mkv", "series", "MobLand", "s02"},
		{"Plus.belle.la.vie.encore.plus.belle.S03E123", "mylen", "Plus.Belle.La.Vie.Encore.Plus.Belle", "s03"},
		{"élan__de. nuit - 2x09.mkv", "series", "Élan.De.Nuit", "s02"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			destination := filepath.Join(base, tt.root, tt.title, tt.season)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				info, err := os.Stat(destination)
				if err != nil || !info.IsDir() {
					t.Errorf("destination was not a directory at RPC time: %v", err)
				}
				var request struct {
					Method    string        `json:"method"`
					Arguments moveArguments `json:"arguments"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Method != "torrent-set-location" || request.Arguments.Location != destination || !request.Arguments.Move {
					t.Errorf("unexpected RPC payload: %+v", request)
				}
				io.WriteString(w, `{"result":"success"}`)
			}))
			defer server.Close()
			cfg := loadExampleConfig(t)
			cfg.RPCURL = server.URL
			for i, rule := range cfg.Rules {
				cfg.Rules[i].Destination = filepath.Join(base, filepath.Base(rule.Destination))
			}
			torrent := Torrent{Name: tt.name, Hash: testHash}
			for range 2 {
				if err := Run(context.Background(), cfg, torrent, Client{}, false, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			torrent.Dir = destination + "/"
			if err := Run(context.Background(), cfg, torrent, Client{}, false, io.Discard); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 2 {
				t.Fatalf("RPC calls = %d, want 2", calls.Load())
			}
		})
	}
}

func TestSeriesPreviewAndInvalidNames(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"result":"success"}`)
	}))
	defer server.Close()
	root := filepath.Join(t.TempDir(), "missing-series-root")
	cfg := testConfig(t, server.URL, root)
	cfg.Rules[1].SeriesSubdirs = true
	var output bytes.Buffer
	err := Run(context.Background(), cfg, Torrent{Name: "my_Show.S01E02", Hash: testHash}, Client{}, true, &output)
	if err != nil || !strings.Contains(output.String(), filepath.Join(root, "My.Show", "s01")) {
		t.Fatalf("dry-run output = %q, error = %v", output.String(), err)
	}
	for _, name := range []string{"../Show.S01E02", `Show\Other.S01E02`, "Show\x00.S01E02", "...S01E02", "S01E02", "Show.S01E02extra"} {
		err := Run(context.Background(), cfg, Torrent{Name: name, Hash: testHash}, Client{}, false, io.Discard)
		if err == nil {
			t.Errorf("expected series name error for %q", name)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("series root should still be absent, stat error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("unexpected RPC calls: %d", calls.Load())
	}
}

func TestSeriesDirectoryCollisions(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"result":"success"}`)
	}))
	defer server.Close()
	for _, component := range []string{"Show", "Show/s01"} {
		for _, kind := range []string{"file", "symlink"} {
			t.Run(component+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, component)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					if err := os.Symlink(t.TempDir(), path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
				cfg := testConfig(t, server.URL, root)
				cfg.Rules[1].SeriesSubdirs = true
				err := Run(context.Background(), cfg, Torrent{Name: "Show.S01E02", Hash: testHash}, Client{}, false, io.Discard)
				if err == nil || !strings.Contains(err.Error(), "not a directory") {
					t.Fatalf("expected directory error, got %v", err)
				}
			})
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("unexpected RPC calls: %d", calls.Load())
	}
}
