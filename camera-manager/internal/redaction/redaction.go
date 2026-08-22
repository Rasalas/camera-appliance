package redaction

import (
	"regexp"
	"strings"
)

// Patterns are compiled once at package init; Text() runs on hot paths such as
// support bundle generation.
var (
	// Matches credentials embedded in URL userinfo. The username may be empty
	// ("rtsp://:password@host") and must still be redacted.
	credentialPattern = regexp.MustCompile(`(?i)\b(rtsp|https?)://([^:/@\s]*):([^@\s]+)@`)
	// Matches secrets passed in query parameters of any URL inside text.
	querySecretPattern = regexp.MustCompile(`(?i)([?&](?:token|pwd|pass|password|secret|api_?key|authorization)=)[^&\s]+`)
	// Matches KEY=value shapes for known secret-bearing keys.
	keyValuePatterns = compileKeyValuePatterns([]string{
		"TAPO_CAMERA_PASSWORD", "ADMIN_SESSION_SECRET", "PASSWORD", "SECRET",
	})
)

func compileKeyValuePatterns(keys []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(keys))
	for _, key := range keys {
		re := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(key) + `\s*=\s*)[^\s]+`)
		patterns = append(patterns, re)
	}
	return patterns
}

func URL(raw string) string {
	return credentialPattern.ReplaceAllString(raw, `${1}://${2}:******@`)
}

func Text(value string) string {
	value = credentialPattern.ReplaceAllString(value, `${1}://${2}:******@`)
	value = querySecretPattern.ReplaceAllString(value, `${1}******`)
	for _, re := range keyValuePatterns {
		value = re.ReplaceAllString(value, `${1}******`)
	}
	return strings.TrimSpace(value)
}
