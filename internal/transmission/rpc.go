package transmission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const sessionHeader = "X-Transmission-Session-Id"

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

func (c Client) Move(ctx context.Context, hash, destination string) error {
	body, err := json.Marshal(struct {
		Method    string        `json:"method"`
		Arguments moveArguments `json:"arguments"`
	}{
		Method:    "torrent-set-location",
		Arguments: moveArguments{IDs: []string{hash}, Location: destination, Move: true},
	})
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
	for attempt := range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create RPC request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if sessionID != "" {
			req.Header.Set(sessionHeader, sessionID)
		}
		if c.Username != "" || c.Password != "" {
			req.SetBasicAuth(c.Username, c.Password)
		}
		resp, err := copyClient.Do(req)
		if err != nil {
			return fmt.Errorf("request torrent relocation (check Transmission before retrying): %w", err)
		}
		if resp.StatusCode == http.StatusConflict && attempt == 0 {
			sessionID = resp.Header.Get(sessionHeader)
			resp.Body.Close()
			if sessionID == "" {
				return fmt.Errorf("RPC session challenge has no %s header", sessionHeader)
			}
			continue
		}
		err = readResponse(resp)
		resp.Body.Close()
		return err
	}
	return fmt.Errorf("RPC session negotiation failed")
}

func readResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Transmission RPC returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Result string `json:"result"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("decode RPC response: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("RPC response must contain exactly one JSON object")
	}
	if result.Result != "success" {
		return fmt.Errorf("Transmission RPC failed: %q", result.Result)
	}
	return nil
}
