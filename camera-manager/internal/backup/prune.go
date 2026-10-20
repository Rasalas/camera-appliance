package backup

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

const (
	KeepBackups           = 5
	KeepRollbackSnapshots = 3
	KeepSupportBundles    = 3
)

// Retention identifies generated entries by exact name so releases, manual
// files and unknown data in the backup directory are never pruned.
type Retention struct {
	pattern *regexp.Regexp
	dirs    bool
}

var (
	BackupArchives    = Retention{pattern: regexp.MustCompile(`^camera-appliance-(\d{8}-\d{6}(?:\.\d+)?)\.tar\.gz$`)}
	RollbackSnapshots = Retention{pattern: regexp.MustCompile(`^rollback-(\d{8}-\d{6}(?:\.\d+)?)$`), dirs: true}
	SupportBundles    = Retention{pattern: regexp.MustCompile(`^support-bundle-(\d{8}-\d{6})\.tar\.gz$`)}
)

// Prune keeps the newest keep entries plus every protected path and returns
// what it removed. It continues after a failed removal.
func Prune(dir string, kind Retention, keep int, protected ...string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	protect := map[string]bool{}
	for _, path := range protected {
		if path != "" {
			protect[filepath.Clean(path)] = true
		}
	}
	type candidate struct{ path, stamp string }
	var found []candidate
	for _, entry := range entries {
		match := kind.pattern.FindStringSubmatch(entry.Name())
		if match == nil || entry.IsDir() != kind.dirs || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		found = append(found, candidate{filepath.Join(dir, entry.Name()), match[1]})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].stamp > found[j].stamp })
	var removed []string
	var errs []error
	for i, item := range found {
		if i < keep || protect[item.path] {
			continue
		}
		if err := os.RemoveAll(item.path); err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, item.path)
	}
	return removed, errors.Join(errs...)
}
