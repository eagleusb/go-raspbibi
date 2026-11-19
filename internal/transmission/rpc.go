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
)

const sessionHeader = "X-Transmission-Session-Id"
const DefaultRPCURL = "http://127.0.0.1:9091/transmission/rpc"

type moveArguments struct {
	IDs      []string `json:"ids"`
	Location string   `json:"location"`
	Move     bool     `json:"move"`
}

// Client uses the legacy RPC protocol supported by Transmission 4.1.0 beta2.
// It delegates relocation to Transmission, which tracks the new data location.
type Client struct {
	URL      string
	Username string
	Password string
	HTTP     *http.Client
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
	return Client{URL: endpoint, Username: username, Password: password}, nil
}

func (c Client) Move(ctx context.Context, hash, destination string) error {
	return c.call(ctx, "torrent-set-location", moveArguments{IDs: []string{hash}, Location: destination, Move: true}, nil)
}

func (c Client) DownloadDir(ctx context.Context, hash string) (string, error) {
	args := struct {
		IDs    []string `json:"ids"`
		Fields []string `json:"fields"`
	}{IDs: []string{hash}, Fields: []string{"downloadDir"}}
	var result struct {
		Torrents []struct {
			DownloadDir string `json:"downloadDir"`
		} `json:"torrents"`
	}
	if err := c.call(ctx, "torrent-get", args, &result); err != nil {
		return "", err
	}
	if len(result.Torrents) != 1 || !filepath.IsAbs(result.Torrents[0].DownloadDir) {
		return "", fmt.Errorf("Transmission RPC must return one torrent with an absolute downloadDir")
	}
	return result.Torrents[0].DownloadDir, nil
}

func (c Client) call(ctx context.Context, method string, args, result any) error {
	body, err := json.Marshal(struct {
		Method    string `json:"method"`
		Arguments any    `json:"arguments"`
	}{Method: method, Arguments: args})
	if err != nil {
		return err
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}
	// Do not forward a hook's RPC request or credentials to a redirect target.
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var sessionID string
	authenticated := c.Username != "" || c.Password != ""
	for attempt := range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create RPC request: %w", err)
		}
		if req.URL.User != nil {
			return fmt.Errorf("RPC URL must not contain credentials")
		}
		req.Header.Set("Content-Type", "application/json")
		if sessionID != "" {
			req.Header.Set(sessionHeader, sessionID)
		}
		if authenticated {
			host := req.URL.Hostname()
			loopback := req.URL.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback())
			if req.URL.Scheme != "https" && !loopback {
				return fmt.Errorf("HTTPS is required for authenticated non-loopback RPC")
			}
			req.SetBasicAuth(c.Username, c.Password)
		}
		resp, err := copyClient.Do(req)
		if err != nil {
			return fmt.Errorf("Transmission RPC %s request: %w", method, err)
		}
		if resp.StatusCode == http.StatusConflict && attempt == 0 {
			sessionID = resp.Header.Get(sessionHeader)
			resp.Body.Close()
			if sessionID == "" {
				return fmt.Errorf("RPC session challenge has no %s header", sessionHeader)
			}
			continue
		}
		err = readResponse(resp, result)
		resp.Body.Close()
		return err
	}
	return fmt.Errorf("RPC session negotiation failed")
}

func readResponse(resp *http.Response, result any) error {
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Transmission RPC returned HTTP %d", resp.StatusCode)
	}
	var response struct {
		Result    string          `json:"result"`
		Arguments json.RawMessage `json:"arguments"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode RPC response: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("RPC response must contain exactly one JSON object")
	}
	if response.Result != "success" {
		return fmt.Errorf("Transmission RPC failed: %q", response.Result)
	}
	if result != nil {
		if err := json.Unmarshal(response.Arguments, result); err != nil {
			return fmt.Errorf("decode RPC response arguments: %w", err)
		}
	}
	return nil
}
