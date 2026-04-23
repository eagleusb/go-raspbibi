package utility

import (
	"strings"
	"unicode"
)

// ReplaceCharacters applies punctuation-to-dot/space replacements on a filename.
func ReplaceCharacters(name string) string {
	replacer := strings.NewReplacer(
		" ", ".",
		"(", ".",
		")", ".",
		"[", ".",
		"]", ".",
		",", " ",
	)
	return replacer.Replace(name)
}

// FilterUnicode keeps only letters, digits, dots, spaces, and hyphens.
func FilterUnicode(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == ' ' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
