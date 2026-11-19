// Package transmission routes torrents and removes expired torrent entries.
package transmission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/hekmon/transmissionrpc/v3"
)

const sessionHeader = "X-Transmission-Session-Id"

// DefaultRPCURL is the local Transmission daemon's default RPC endpoint.
const DefaultRPCURL = "http://127.0.0.1:9091/transmission/rpc"
const maxRPCResponseBytes = 1 << 20

// Client wraps the Transmission RPC operations used by routing and cleanup.
type Client struct {
	rpc *transmissionrpc.Client
}

// NewClient validates the endpoint or uses the default local RPC URL.
func NewClient(endpoint, username, password string) (Client, error) {
	if endpoint == "" {
		endpoint = DefaultRPCURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.ContainsRune(endpoint, '#') {
		return Client{}, fmt.Errorf("TRANSMISSION_RPC_URL must be an HTTP(S) URL without credentials, query, or fragment")
	}
	httpClient := &http.Client{
		Transport:     rpcTransport{username: username, password: password},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	rpc, err := transmissionrpc.New(parsed, &transmissionrpc.Config{CustomClient: httpClient})
	if err != nil {
		return Client{}, fmt.Errorf("initialize Transmission RPC client: %w", err)
	}
	return Client{rpc: rpc}, nil
}

// Move asks Transmission to relocate the torrent identified by hash.
func (c Client) Move(ctx context.Context, hash, destination string) error {
	return c.rpc.TorrentSetLocationHash(ctx, hash, destination, true)
}

// DownloadDir returns the absolute download directory for the given torrent.
func (c Client) DownloadDir(ctx context.Context, hash string) (string, error) {
	torrents, err := c.rpc.TorrentGetHashes(ctx, []string{"downloadDir"}, []string{hash})
	if err != nil {
		return "", err
	}
	if len(torrents) != 1 || torrents[0].DownloadDir == nil || !filepath.IsAbs(*torrents[0].DownloadDir) {
		return "", fmt.Errorf("Transmission RPC must return one torrent with an absolute downloadDir")
	}
	return *torrents[0].DownloadDir, nil
}

type rpcTransport struct {
	username string
	password string
}

type numericTorrentIDsKey struct{}

func (t rpcTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.username != "" || t.password != "" {
		host := req.URL.Hostname()
		if req.URL.Scheme != "https" && host != "localhost" && !net.ParseIP(host).IsLoopback() {
			return nil, fmt.Errorf("HTTPS is required for authenticated non-loopback RPC")
		}
		req = req.Clone(req.Context())
		req.SetBasicAuth(t.username, t.password)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusConflict {
		// Cleanup's numeric IDs are valid only within the session that listed
		// them. Routing uses hashes and can retry against a new session.
		if req.Header.Get(sessionHeader) != "" && req.Context().Value(numericTorrentIDsKey{}) == true {
			resp.Body.Close()
			return nil, fmt.Errorf("Transmission RPC session was rejected")
		}
		if resp.Header.Get(sessionHeader) == "" {
			resp.Body.Close()
			return nil, fmt.Errorf("RPC session challenge has no %s header", sessionHeader)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRPCResponseBytes+1))
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read RPC response: %w", err)
	}
	if len(body) > maxRPCResponseBytes {
		return nil, fmt.Errorf("Transmission RPC response exceeds %d bytes", maxRPCResponseBytes)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("Transmission RPC returned invalid JSON")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}
