package redaction

import (
	"regexp"
	"strings"
)

var credentialPattern = regexp.MustCompile(`(?i)(rtsp|http|https)://([^:/@\s]+):([^@\s]+)@`)

func URL(raw string) string {
	return credentialPattern.ReplaceAllString(raw, `${1}://${2}:******@`)
}

func Text(value string) string {
	value = credentialPattern.ReplaceAllString(value, `${1}://${2}:******@`)
	for _, key := range []string{"TAPO_CAMERA_PASSWORD", "ADMIN_SESSION_SECRET", "PASSWORD", "SECRET"} {
		re := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(key) + `\s*=\s*)[^\s]+`)
		value = re.ReplaceAllString(value, `${1}******`)
	}
	return strings.TrimSpace(value)
}
