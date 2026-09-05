package streamrouting

import (
	"fmt"
	"strings"
)

func orderStreamPaths(paths []StreamPath, policy, lastPathID string) []StreamPath {
	var out []StreamPath
	add := func(match func(StreamPath) bool) {
		for _, path := range paths {
			if match(path) && !containsPath(out, path.ID) {
				out = append(out, path)
			}
		}
	}
	if lastPathID != "" && policy == PathPolicyAuto {
		add(func(path StreamPath) bool { return path.ID == lastPathID })
	}
	switch policy {
	case PathPolicyDirectOnly:
		add(func(path StreamPath) bool { return path.Kind == PathKindDirect })
	case PathPolicyRelayOnly:
		add(func(path StreamPath) bool { return path.Kind == PathKindRelay })
	case PathPolicyPreferRelay:
		add(func(path StreamPath) bool { return path.Kind == PathKindRelay })
		add(func(path StreamPath) bool { return path.Kind == PathKindDirect })
	default:
		add(func(path StreamPath) bool { return path.Kind == PathKindDirect })
		add(func(path StreamPath) bool { return path.Kind == PathKindRelay })
	}
	return out
}

func containsPath(paths []StreamPath, id string) bool {
	for _, path := range paths {
		if path.ID == id {
			return true
		}
	}
	return false
}

func Policy(deviceID string, settings map[string]string) string {
	policy := strings.TrimSpace(settings["camera.path_policy."+deviceID])
	switch policy {
	case PathPolicyAuto, PathPolicyDirectOnly, PathPolicyRelayOnly, PathPolicyPreferDirect, PathPolicyPreferRelay:
		return policy
	default:
		return PathPolicyAuto
	}
}

func selectStableStreamPath(paths []StreamPath, activePathID string, cfg PathStabilityConfig) (int, string) {
	firstOK := firstPathIndex(paths, func(path StreamPath) bool { return path.State == "ok" })
	activeIndex := pathIndex(paths, activePathID)
	if activePathID == "" {
		if firstOK >= 0 {
			return firstOK, "initial_selection"
		}
		return -1, ""
	}
	if activeIndex < 0 {
		if firstOK >= 0 {
			return firstOK, "policy_selected"
		}
		return -1, ""
	}
	active := paths[activeIndex]
	if active.State != "ok" {
		if active.FailureCount < cfg.FailThreshold {
			return activeIndex, ""
		}
		if firstOK >= 0 && firstOK != activeIndex {
			return firstOK, fmt.Sprintf("active_failed_%d", cfg.FailThreshold)
		}
		return activeIndex, ""
	}
	preferredIndex := firstPathIndex(paths, func(path StreamPath) bool { return path.State == "ok" })
	if preferredIndex >= 0 && preferredIndex != activeIndex && preferredIndex < activeIndex {
		if paths[preferredIndex].SuccessCount >= cfg.RecoveryThreshold {
			return preferredIndex, fmt.Sprintf("preferred_recovered_%d", cfg.RecoveryThreshold)
		}
	}
	return activeIndex, ""
}

func annotateStreamPathStability(paths []StreamPath, activePathID string, cfg PathStabilityConfig) {
	activeIndex := pathIndex(paths, activePathID)
	for i := range paths {
		path := &paths[i]
		switch {
		case path.ID == activePathID && path.State == "ok":
			path.Stability = "stable"
			path.StabilityMessage = "Aktiver Pfad ist stabil."
		case path.ID == activePathID && path.State != "ok" && path.FailureCount < cfg.FailThreshold:
			path.Stability = "failing"
			path.StabilityMessage = fmt.Sprintf("Aktiver Pfad wird gehalten: %d/%d Fehler.", path.FailureCount, cfg.FailThreshold)
		case path.ID == activePathID && path.State != "ok":
			path.Stability = "unstable"
			path.StabilityMessage = fmt.Sprintf("Aktiver Pfad hat die Fehlerschwelle erreicht: %d/%d.", path.FailureCount, cfg.FailThreshold)
		case path.State == "ok" && activeIndex >= 0 && i < activeIndex && path.SuccessCount < cfg.RecoveryThreshold:
			path.Stability = "warming"
			path.StabilityMessage = fmt.Sprintf("Pfad wartet auf stabile Erholung: %d/%d Erfolge.", path.SuccessCount, cfg.RecoveryThreshold)
		case path.State == "ok":
			path.Stability = "stable"
			path.StabilityMessage = "Pfad ist erreichbar."
		default:
			path.Stability = "failed"
			path.StabilityMessage = "Pfad ist nicht erreichbar."
		}
	}
}

func firstPathIndex(paths []StreamPath, match func(StreamPath) bool) int {
	for i, path := range paths {
		if match(path) {
			return i
		}
	}
	return -1
}

func pathIndex(paths []StreamPath, id string) int {
	if id == "" {
		return -1
	}
	for i, path := range paths {
		if path.ID == id {
			return i
		}
	}
	return -1
}
