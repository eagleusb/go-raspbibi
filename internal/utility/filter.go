package utility

import "regexp"

// Pattern holds a pre-compiled regular expression for efficient reuse.
type Pattern struct {
	Regex string
	regex *regexp.Regexp
}

// NewPattern compiles the given expression into a Pattern.
// Panics on invalid regex expressions (fail fast).
func NewPattern(expr string) Pattern {
	return Pattern{
		Regex: expr,
		regex: regexp.MustCompile(expr),
	}
}

// HasExtension reports whether the file extension matches any in the allowed list.
// Extensions should include the dot prefix (e.g. ".mkv", ".mp4").
func HasExtension(ext string, allowed []string) bool {
	for _, a := range allowed {
		if ext == a {
			return true
		}
	}
	return false
}

// MatchAny returns true if the string matches any of the given patterns.
func MatchAny(s string, patterns []Pattern) bool {
	for _, p := range patterns {
		if p.regex.MatchString(s) {
			return true
		}
	}
	return false
}
