package transmission

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultRPCURL = "http://127.0.0.1:9091/transmission/rpc"

// Config contains ordered routing rules. An empty destination explicitly keeps
// a matching torrent in its existing location. Unmatched torrents also stay put.
type Config struct {
	RPCURL string `json:"rpc_url"`
	Rules  []Rule `json:"rules"`
}

type Rule struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Destination string `json:"destination,omitempty"`
	regex       *regexp.Regexp
}

// LoadConfig validates the whole file before any filesystem or RPC operations.
func LoadConfig(r io.Reader) (Config, error) {
	cfg := Config{RPCURL: DefaultRPCURL}
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode routing config: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, fmt.Errorf("routing config must contain exactly one JSON object")
	}
	endpoint, err := url.Parse(cfg.RPCURL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return Config{}, fmt.Errorf("rpc_url must be an HTTP(S) URL without credentials, query, or fragment")
	}
	if len(cfg.Rules) == 0 {
		return Config{}, fmt.Errorf("routing config must contain at least one rule")
	}
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		if strings.TrimSpace(rule.Name) == "" || strings.TrimSpace(rule.Pattern) == "" {
			return Config{}, fmt.Errorf("rule %d requires a name and pattern", i+1)
		}
		if rule.Destination != "" && (!filepath.IsAbs(rule.Destination) || strings.ContainsRune(rule.Destination, 0)) {
			return Config{}, fmt.Errorf("rule %q destination must be an absolute path", rule.Name)
		}
		rule.regex, err = regexp.Compile(rule.Pattern)
		if err != nil {
			return Config{}, fmt.Errorf("rule %q pattern: %w", rule.Name, err)
		}
	}
	return cfg, nil
}

// Match returns the first rule matching the torrent name.
func (cfg Config) Match(name string) (Rule, bool) {
	for _, rule := range cfg.Rules {
		if rule.regex != nil && rule.regex.MatchString(name) {
			return rule, true
		}
	}
	return Rule{}, false
}
