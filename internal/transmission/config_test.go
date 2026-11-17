package transmission

import (
	"os"
	"strings"
	"testing"
)

func TestExampleRules(t *testing.T) {
	f, err := os.Open("../../examples/transmission-route.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, rule, destination string
		matched                 bool
	}{
		{"Show.S01E02.1080p", "series", "/mnt/sda/séries", true},
		{"Show.s02.2024.Complete", "series", "/mnt/sda/séries", true},
		{"Show.1x02.mkv", "series", "/mnt/sda/séries", true},
		{"Show S1E2", "series", "/mnt/sda/séries", true},
		{"Film.2024.1080p", "movies", "/mnt/sdb/movies-hd", true},
		{"Film (1999)", "movies", "/mnt/sdb/movies-hd", true},
		{"Custom.Show.S01E02.2024", "custom", "", true},
		{"Film.custom.2024", "custom", "", true},
		{"Unsorted.Download", "", "", false},
		{"Release.1080p", "", "", false},
		{"Film.120240", "", "", false},
		{"Class01E02.notes", "", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rule, matched := cfg.Match(tt.name)
			if matched != tt.matched || rule.Name != tt.rule || rule.Destination != tt.destination {
				t.Fatalf("Match = (%+v, %v), want rule %q, destination %q, matched %v", rule, matched, tt.rule, tt.destination, tt.matched)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		`{`,
		`null`,
		`{"rules":[]}`,
		`{"rulez":[]}`,
		`{"rules":[{"name":"x","pattern":"["}]}`,
		`{"rules":[{"name":"x","pattern":""}]}`,
		`{"rules":[{"name":"","pattern":"x"}]}`,
		`{"rules":[{"name":"x","pattern":"x","destination":"relative"}]}`,
		`{"rules":[{"name":"x","pattern":"x","destination":"/bad\u0000path"}]}`,
		`{"rpc_url":"file:///tmp/rpc","rules":[{"name":"x","pattern":"x"}]}`,
		`{"rpc_url":"http://user:secret@localhost/rpc","rules":[{"name":"x","pattern":"x"}]}`,
		`{"rpc_url":"http://localhost/rpc?password=secret","rules":[{"name":"x","pattern":"x"}]}`,
		`{"rules":[{"name":"x","pattern":"x"}]} {}`,
		`{"rules":[{"name":"x","pattern":"x"}]} garbage`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := LoadConfig(strings.NewReader(input)); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestDefaultRPCURLAndSkipRule(t *testing.T) {
	cfg, err := LoadConfig(strings.NewReader(`{"rules":[{"name":"custom","pattern":".*"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RPCURL != DefaultRPCURL {
		t.Fatalf("RPCURL = %q", cfg.RPCURL)
	}
	rule, matched := cfg.Match("anything")
	if !matched || rule.Destination != "" {
		t.Fatal("rule without a destination should match and keep the existing location")
	}
}
