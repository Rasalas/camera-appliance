package streamrouting

import (
	"strings"
)

func ProbeHostForEndpoint(host string) string {
	if strings.EqualFold(strings.TrimSpace(host), "host.docker.internal") {
		return "127.0.0.1"
	}
	return host
}

func ProbeDiagnostic(port string, err error) string {
	if strings.TrimSpace(port) == "" {
		port = "554"
	}
	if err == nil {
		return "RTSP-Port " + port + " ist erreichbar."
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "refused") {
		return "RTSP-Port " + port + " lehnt Verbindungen ab."
	}
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
		return "RTSP-Port " + port + " antwortet nicht."
	}
	return "RTSP-Port " + port + " ist nicht erreichbar."
}
