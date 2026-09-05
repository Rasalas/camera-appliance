package streamrouting

import (
	"strconv"
	"strings"
	"time"
)

type streamPathCounters struct {
	SuccessCount  int
	FailureCount  int
	LastSuccessAt string
	LastFailureAt string
}

func StabilityConfig(settings map[string]string) PathStabilityConfig {
	return PathStabilityConfig{
		FailThreshold:     intSetting(settings, PathFailThresholdKey, defaultPathFailThreshold, 1, 20),
		RecoveryThreshold: intSetting(settings, PathRecoveryThresholdKey, defaultPathRecoveryThreshold, 1, 20),
	}
}

func streamPathCountersFromSettings(settings map[string]string, deviceID, pathID string) streamPathCounters {
	return streamPathCounters{
		SuccessCount:  intSetting(settings, pathStateKey(deviceID, pathID, "success_count"), 0, 0, 1000000),
		FailureCount:  intSetting(settings, pathStateKey(deviceID, pathID, "failure_count"), 0, 0, 1000000),
		LastSuccessAt: strings.TrimSpace(settings[pathStateKey(deviceID, pathID, "last_success_at")]),
		LastFailureAt: strings.TrimSpace(settings[pathStateKey(deviceID, pathID, "last_failure_at")]),
	}
}

func updateStreamPathCounters(counters streamPathCounters, state string, checkedAt time.Time) streamPathCounters {
	if state == "ok" {
		counters.SuccessCount++
		counters.FailureCount = 0
		counters.LastSuccessAt = checkedAt.Format(time.RFC3339)
		return counters
	}
	counters.FailureCount++
	counters.SuccessCount = 0
	counters.LastFailureAt = checkedAt.Format(time.RFC3339)
	return counters
}

func streamPathCounterValues(deviceID, pathID string, counters streamPathCounters) map[string]string {
	values := map[string]string{
		pathStateKey(deviceID, pathID, "success_count"): strconv.Itoa(counters.SuccessCount),
		pathStateKey(deviceID, pathID, "failure_count"): strconv.Itoa(counters.FailureCount),
	}
	if counters.LastSuccessAt != "" {
		values[pathStateKey(deviceID, pathID, "last_success_at")] = counters.LastSuccessAt
	}
	if counters.LastFailureAt != "" {
		values[pathStateKey(deviceID, pathID, "last_failure_at")] = counters.LastFailureAt
	}
	return values
}

func applyStreamPathCounters(path *StreamPath, counters streamPathCounters) {
	path.SuccessCount = counters.SuccessCount
	path.FailureCount = counters.FailureCount
	path.LastSuccessAt = counters.LastSuccessAt
	path.LastFailureAt = counters.LastFailureAt
}

func pathStateKey(deviceID, pathID, name string) string {
	return pathStateKeyPrefix + safePathStateID(deviceID) + "." + safePathStateID(pathID) + "." + name
}

func safePathStateID(value string) string {
	var out strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('_')
	}
	cleaned := strings.Trim(out.String(), "_-")
	if cleaned == "" {
		return "path"
	}
	return cleaned
}

func intSetting(settings map[string]string, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(settings[key])
	value, err := strconv.Atoi(raw)
	if err != nil {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
