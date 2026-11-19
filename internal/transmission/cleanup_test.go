package transmission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

type cleanupRequest struct {
	Method    string `json:"method"`
	Tag       int    `json:"tag"`
	Arguments struct {
		Fields          []string `json:"fields"`
		IDs             []int64  `json:"ids"`
		DeleteLocalData *bool    `json:"delete-local-data"`
	} `json:"arguments"`
}

func TestCleanup(t *testing.T) {
	const idList = `{"result":"success","arguments":{"torrents":[
		{"id":0},{"id":2},{"id":3},{"id":4},{"id":5},{"id":6},{"id":7},{"id":8},{"id":9},{"id":10},{"id":-1},{}
	]}}`
	const torrentList = `{"result":"success","arguments":{"torrents":[
		{"id":0,"name":"at limit","leftUntilDone":0,"secondsSeeding":864000,"status":6},
		{"id":2,"name":"above limit","leftUntilDone":0,"secondsSeeding":864001,"status":0},
		{"id":3,"name":"queued","leftUntilDone":0,"secondsSeeding":864001,"status":5},
		{"id":4,"name":"below limit","leftUntilDone":0,"secondsSeeding":863999,"status":6},
		{"id":5,"name":"rounded completion","percentDone":1,"leftUntilDone":1,"secondsSeeding":864000,"status":0},
		{"id":6,"name":"verifying","leftUntilDone":0,"secondsSeeding":864000,"status":2},
		{"id":7,"name":"downloading","leftUntilDone":0,"secondsSeeding":864000,"status":4},
		{"id":8,"name":"missing status","leftUntilDone":0,"secondsSeeding":864000},
		{"id":9,"name":"missing remaining bytes","percentDone":1,"secondsSeeding":864000,"status":6},
		{"id":10,"name":"missing seeding time","leftUntilDone":0,"status":6},
		{"id":-1,"name":"invalid ID","leftUntilDone":0,"secondsSeeding":864000,"status":6},
		{"name":"missing ID","leftUntilDone":0,"secondsSeeding":864000,"status":6}
	]}}`
	wantLookupIDs := []int64{0, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	wantRemovalIDs := []int64{0, 2, 3}
	for _, tt := range []struct {
		name, ids, batch, removal, wantError string
		dryRun, sessionChange                bool
		wantRemovals                         int32
	}{
		{name: "remove", batch: torrentList, removal: `{"result":"success"}`, wantRemovals: 1},
		{name: "dry run", batch: torrentList, dryRun: true},
		{name: "empty list", ids: `{"result":"success","arguments":{"torrents":[]}}`},
		{name: "no matches", batch: `{"result":"success","arguments":{"torrents":[]}}`},
		{name: "missing list", ids: `{"result":"success","arguments":{}}`, wantError: "no torrents array"},
		{name: "missing batch", batch: `{"result":"success","arguments":{}}`, wantError: "no torrents array"},
		{name: "malformed batch", batch: `{"result":"success","arguments":{"torrents":[{"leftUntilDone":"unknown"}]}}`, wantError: "cannot unmarshal"},
		{name: "oversized batch", batch: torrentList + strings.Repeat(" ", maxRPCResponseBytes), wantError: "response exceeds 1048576 bytes"},
		{name: "list error", ids: `{"result":"list denied"}`, wantError: "list denied"},
		{name: "remove error", batch: torrentList, removal: `{"result":"remove denied"}`, wantError: "remove denied", wantRemovals: 1},
		{name: "session changes before removal", batch: torrentList, removal: `{"result":"success"}`, sessionChange: true, wantError: "RPC session was rejected", wantRemovals: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var listCalls, batchCalls, removeCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				token := r.Header.Get(sessionHeader)
				if token == "" {
					w.Header().Set(sessionHeader, "test-session")
					w.WriteHeader(http.StatusConflict)
					return
				}
				var request cleanupRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode RPC request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				args := request.Arguments
				switch {
				case request.Method == "torrent-get" && args.IDs == nil:
					listCalls.Add(1)
					if !slices.Equal(args.Fields, []string{"id"}) {
						t.Errorf("listing fields = %v, want only id", args.Fields)
					}
					reply := tt.ids
					if reply == "" {
						reply = idList
					}
					writeRPCReply(w, request.Tag, reply)
				case request.Method == "torrent-get":
					batchCalls.Add(1)
					wantFields := []string{"id", "leftUntilDone", "secondsSeeding", "status"}
					if tt.dryRun {
						wantFields = append(wantFields, "name")
					}
					if !slices.Equal(args.Fields, wantFields) || !slices.Equal(args.IDs, wantLookupIDs) {
						t.Errorf("lookup arguments = %+v, want fields %v and IDs %v", args, wantFields, wantLookupIDs)
					}
					writeRPCReply(w, request.Tag, tt.batch)
				case request.Method == "torrent-remove":
					removeCalls.Add(1)
					if !slices.Equal(args.IDs, wantRemovalIDs) {
						t.Errorf("removal IDs = %v, want %v", args.IDs, wantRemovalIDs)
					}
					if args.DeleteLocalData == nil || *args.DeleteLocalData {
						t.Error("torrent-remove must explicitly set delete-local-data to false")
					}
					if tt.sessionChange && token == "test-session" {
						w.Header().Set(sessionHeader, "replacement-session")
						w.WriteHeader(http.StatusConflict)
						return
					}
					writeRPCReply(w, request.Tag, tt.removal)
				default:
					t.Errorf("unexpected RPC method %q", request.Method)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()

			var output bytes.Buffer
			client := mustRPCClient(t, server.URL, "", "")
			err := Cleanup(t.Context(), client, tt.dryRun, &output)
			if tt.wantError == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
			var wantBatches int32
			if tt.batch != "" {
				wantBatches = 1
			}
			if listCalls.Load() != 1 || batchCalls.Load() != wantBatches || removeCalls.Load() != tt.wantRemovals {
				t.Errorf("RPC calls = %d list, %d batch, %d remove, want 1 list, %d batch, %d remove",
					listCalls.Load(), batchCalls.Load(), removeCalls.Load(), wantBatches, tt.wantRemovals)
			}
			if tt.dryRun {
				preview := output.String()
				if strings.Count(preview, "[DRY-RUN]") != len(wantRemovalIDs) ||
					!strings.Contains(preview, `"at limit" (ID 0) after 864000 seconds`) ||
					!strings.Contains(preview, `"above limit" (ID 2) after 864001 seconds`) ||
					!strings.Contains(preview, `"queued" (ID 3) after 864001 seconds`) {
					t.Errorf("unexpected dry-run output: %q", preview)
				}
			}
		})
	}
}

func TestCleanupScansLargeInventoryInBatches(t *testing.T) {
	const torrentCount = 250
	name := strings.Repeat("x", 5000)

	var batches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request cleanupRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method != "torrent-get" {
			t.Errorf("unexpected dry-run RPC method %q", request.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		torrents := make([]map[string]any, 0, torrentCount)
		if request.Arguments.IDs == nil && slices.Equal(request.Arguments.Fields, []string{"id"}) {
			for id := range torrentCount {
				torrents = append(torrents, map[string]any{"id": id})
			}
		} else {
			batches.Add(1)
			ids := request.Arguments.IDs
			if len(ids) == 0 || len(ids) > cleanupBatchSize {
				t.Errorf("invalid batch size %d", len(ids))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, id := range ids {
				seconds := 863999
				if id == torrentCount-1 {
					seconds++
				}
				torrents = append(torrents, map[string]any{
					"id": id, "name": name, "leftUntilDone": 0, "secondsSeeding": seconds, "status": 6,
				})
			}
		}
		err := json.NewEncoder(w).Encode(map[string]any{
			"tag": request.Tag, "result": "success", "arguments": map[string]any{"torrents": torrents},
		})
		if err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	client := mustRPCClient(t, server.URL, "", "")
	if err := Cleanup(t.Context(), client, true, &output); err != nil {
		t.Fatal(err)
	}
	wantBatches := int32((torrentCount + cleanupBatchSize - 1) / cleanupBatchSize)
	want := fmt.Sprintf("[DRY-RUN] Would remove %q (ID %d) after 864000 seconds of seeding\n", name, torrentCount-1)
	if batches.Load() != wantBatches || output.String() != want {
		t.Fatalf("got %d batches and preview length %d, want %d batches and preview length %d",
			batches.Load(), output.Len(), wantBatches, len(want))
	}
}
