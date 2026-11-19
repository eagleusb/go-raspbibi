package transmission

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	for _, rule := range defaultRules {
		if rule.destination != "" && (!filepath.IsAbs(rule.destination) || strings.ContainsRune(rule.destination, 0)) {
			t.Errorf("rule %q has invalid destination %q", rule.name, rule.destination)
		}
	}
	for _, tt := range []struct {
		name, rule, destination string
		matched                 bool
	}{
		{"Plus.belle.la.vie.encore.plus.belle.S03E123.2024.1080p", "plus-belle-la-vie", "/mnt/sdb/mylen/Plus.belle.la.vie.encore.plus.belle/s03", true},
		{"plus.belle.la.vie.encore.plus.belle.s03e123.mkv", "plus-belle-la-vie", "/mnt/sdb/mylen/Plus.belle.la.vie.encore.plus.belle/s03", true},
		{"Plus.belle.la.vie.encore.plus.belle.S03E12", "series", "/mnt/sda/series", true},
		{"Plus.belle.la.vie.encore.plus.belle.S01E123", "series", "/mnt/sda/series", true},
		{"Plus.belle.la.vie.encore.plus.belle.S10E12", "series", "/mnt/sda/series", true},
		{"PlusXbelleXlaXvieXencoreXplusXbelle.S03E123", "series", "/mnt/sda/series", true},
		{"Other.Plus.belle.la.vie.encore.plus.belle.S03E123", "series", "/mnt/sda/series", true},
		{"Plus.belle.la.vie.encore.plus.belle.S03E123extra", "", "", false},
		{"Show.S01E02.1080p", "series", "/mnt/sda/series", true},
		{"Show.s02.2024.Complete", "series", "/mnt/sda/series", true},
		{"Show.1x02.mkv", "series", "/mnt/sda/series", true},
		{"Show S1E2", "series", "/mnt/sda/series", true},
		{"Film.2024.1080p", "movies", "/mnt/sdb/movies-hd", true},
		{"Film (1999)", "movies", "/mnt/sdb/movies-hd", true},
		{"Unsorted.Download", "", "", false},
		{"Release.1080p", "", "", false},
		{"Film.120240", "", "", false},
		{"Class01E02.notes", "", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rule, matched := matchRoute(defaultRules[:], tt.name)
			if matched != tt.matched || rule.name != tt.rule || rule.destination != tt.destination {
				t.Fatalf("matchRoute = (%+v, %v), want rule %q, destination %q, matched %v", rule, matched, tt.rule, tt.destination, tt.matched)
			}
		})
	}
}
