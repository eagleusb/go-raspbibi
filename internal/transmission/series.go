package transmission

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var seriesName = regexp.MustCompile(`(?i)^(.*?)(?:^|[ ._-]+)(?:s([0-9]{1,2})(?:e[0-9]{1,3})?|([0-9]{1,2})x[0-9]{1,3})(?:[ ._-]|$)`)

func seriesPathComponents(name string) (title, season string, err error) {
	match := seriesName.FindStringSubmatch(name)
	if match == nil {
		return "", "", fmt.Errorf("no season marker in %q", name)
	}
	title = match[1]
	if !utf8.ValidString(title) || strings.ContainsAny(title, `/\`) || strings.ContainsFunc(title, unicode.IsControl) {
		return "", "", fmt.Errorf("invalid show title %q", title)
	}
	words := strings.FieldsFunc(title, func(r rune) bool { return r == '.' || r == ' ' || r == '_' })
	if len(words) == 0 {
		return "", "", fmt.Errorf("missing show title in %q", name)
	}
	for i, word := range words {
		for offset, r := range word {
			if unicode.IsLetter(r) {
				words[i] = word[:offset] + string(unicode.ToUpper(r)) + word[offset+utf8.RuneLen(r):]
				break
			}
		}
	}

	season = match[2]
	if season == "" {
		season = match[3]
	}
	if len(season) == 1 {
		season = "0" + season
	}
	return strings.Join(words, "."), "s" + season, nil
}

func prepareSeriesDestination(root, showDir, seasonDir string) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("create series root %q: %w", root, err)
	}
	for _, dir := range []string{showDir, seasonDir} {
		if err := os.Mkdir(dir, 0755); err != nil && !os.IsExist(err) {
			return fmt.Errorf("create series directory %q: %w", dir, err)
		}
		// Lstat rejects symlinks even when they point to a directory.
		info, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("check series directory %q: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("series destination %q is not a directory", dir)
		}
	}
	return nil
}
