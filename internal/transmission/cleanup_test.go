package transmission

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCleanup(t *testing.T) {
	const (
		atLimit     = "1111111111111111111111111111111111111111"
		aboveLimit  = "2222222222222222222222222222222222222222"
		queued      = "3333333333333333333333333333333333333333"
		torrentList = `{"result":"success","arguments":{"torrents":[
			{"name":"at limit","hashString":"` + atLimit + `","percentDone":1,"secondsSeeding":864000,"status":6},
			{"name":"above limit","hashString":"` + aboveLimit + `","percentDone":1,"secondsSeeding":864001,"status":0},
			{"name":"queued","hashString":"` + queued + `","percentDone":1,"secondsSeeding":864001,"status":5},
			{"name":"below limit","hashString":"4444444444444444444444444444444444444444","percentDone":1,"secondsSeeding":863999,"status":6},
			{"name":"incomplete","hashString":"5555555555555555555555555555555555555555","percentDone":0.9,"secondsSeeding":864000,"status":0},
			{"name":"verifying","hashString":"6666666666666666666666666666666666666666","percentDone":1,"secondsSeeding":864000,"status":2},
			{"name":"downloading","hashString":"7777777777777777777777777777777777777777","percentDone":1,"secondsSeeding":864000,"status":4},
			{"name":"missing status","hashString":"8888888888888888888888888888888888888888","percentDone":1,"secondsSeeding":864000},
			{"name":"missing completion","hashString":"9999999999999999999999999999999999999999","secondsSeeding":864000,"status":6},
			{"name":"missing seeding time","hashString":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","percentDone":1,"status":6},
			{"name":"invalid hash","hashString":"recently-active","percentDone":1,"secondsSeeding":864000,"status":6}
		]}}`
	)
	wantHashes := []string{atLimit, aboveLimit, queued}
	for _, tt := range []struct {
		name, listReply, removeReply, wantError string
		dryRun                                  bool
		wantRemovals                            int32
	}{
		{name: "remove", listReply: torrentList, removeReply: `{"result":"success"}`, wantRemovals: 1},
		{name: "dry run", listReply: torrentList, dryRun: true},
		{name: "empty list", listReply: `{"result":"success","arguments":{"torrents":[]}}`},
		{name: "no matches", listReply: `{"result":"success","arguments":{"torrents":[{"secondsSeeding":1}]}}`},
		{name: "missing list", listReply: `{"result":"success","arguments":{}}`, wantError: "no torrents array"},
		{name: "malformed list", listReply: `{"result":"success","arguments":{"torrents":[{"secondsSeeding":"unknown"}]}}`, wantError: "decode RPC response arguments"},
		{name: "list error", listReply: `{"result":"list denied"}`, wantError: "list denied"},
		{name: "remove error", listReply: torrentList, removeReply: `{"result":"remove denied"}`, wantError: "remove denied", wantRemovals: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var listCalls, removeCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method    string `json:"method"`
					Arguments struct {
						Fields          []string `json:"fields"`
						IDs             []string `json:"ids"`
						DeleteLocalData *bool    `json:"delete-local-data"`
					} `json:"arguments"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode RPC request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				switch request.Method {
				case "torrent-get":
					listCalls.Add(1)
					wantFields := []string{"name", "hashString", "percentDone", "secondsSeeding", "status"}
					if !slices.Equal(request.Arguments.Fields, wantFields) || request.Arguments.IDs != nil {
						t.Errorf("unexpected listing arguments: %+v", request.Arguments)
					}
					io.WriteString(w, tt.listReply)
				case "torrent-remove":
					removeCalls.Add(1)
					if !slices.Equal(request.Arguments.IDs, wantHashes) {
						t.Errorf("removal IDs = %v, want %v", request.Arguments.IDs, wantHashes)
					}
					if request.Arguments.DeleteLocalData == nil || *request.Arguments.DeleteLocalData {
						t.Error("torrent-remove must explicitly set delete-local-data to false")
					}
					io.WriteString(w, tt.removeReply)
				default:
					t.Errorf("unexpected RPC method %q", request.Method)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			var output bytes.Buffer
			err := Cleanup(t.Context(), Client{URL: server.URL}, tt.dryRun, &output)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
			if listCalls.Load() != 1 || removeCalls.Load() != tt.wantRemovals {
				t.Errorf("RPC calls = %d list, %d remove, want 1 list, %d remove", listCalls.Load(), removeCalls.Load(), tt.wantRemovals)
			}
			if tt.dryRun {
				preview := output.String()
				if strings.Count(preview, "[DRY-RUN]") != len(wantHashes) {
					t.Errorf("unexpected dry run output: %q", preview)
				}
				for _, hash := range wantHashes {
					if !strings.Contains(preview, hash) {
						t.Errorf("dry run did not preview %s", hash)
					}
				}
			}
		})
	}
}
