package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// Headroom per filesystem so state, events and logs still fit after the update.
	updateSpaceMargin   uint64 = 256 << 20
	rollbackSpaceMargin uint64 = 32 << 20
	// A build after upstream base images moved measured 3.4–4.2 GB of new cache
	// and image layers; see docs/operations.md before lowering this.
	dockerBuildSpace uint64 = 5 << 30
)

type spaceNeed struct {
	path  string
	bytes uint64
	// build marks the Docker image build; its path is used as is.
	build bool
}

// statFree is replaced in tests.
var statFree = func(path string) (free, device uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	var info syscall.Stat_t
	if err := syscall.Stat(path, &info); err != nil {
		return 0, 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), uint64(info.Dev), nil
}

// checkFreeSpace sums needs per filesystem. A failed statfs skips that check:
// staging already keeps the live install intact on ENOSPC.
func checkFreeSpace(margin uint64, needs ...spaceNeed) error {
	type disk struct {
		path              string
		free, need, build uint64
	}
	var disks []*disk
	byDevice := map[uint64]*disk{}
	for _, n := range needs {
		path := n.path
		if !n.build {
			path = existingParent(path)
		}
		free, device, err := statFree(path)
		if err != nil {
			continue
		}
		d := byDevice[device]
		if d == nil {
			d = &disk{path: path, free: free}
			byDevice[device] = d
			disks = append(disks, d)
		}
		d.need += n.bytes
		if n.build {
			d.build += n.bytes
		}
	}
	for _, d := range disks {
		if d.free >= d.need+margin {
			continue
		}
		detail, hint := "", ""
		if d.build > 0 {
			detail = fmt.Sprintf(" (davon %s für den Docker-Image-Build)", formatSize(d.build))
			hint = ", z. B. alten Build-Cache mit „docker builder prune“,"
		}
		return fmt.Errorf("nicht genug freier Speicherplatz auf %s: benötigt %s%s, frei %s. Bitte Speicher freigeben%s und erneut versuchen; die Installation wurde nicht verändert", d.path, formatSize(d.need+margin), detail, formatSize(d.free), hint)
	}
	return nil
}

// treeSize estimates the disk usage of a copy, rounding files up to 4 KiB blocks.
func treeSize(root string) (uint64, error) {
	var total uint64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel != "." && shouldSkipCopyPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			total += 4096
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += (uint64(info.Size()) + 4095) &^ 4095
		return nil
	})
	return total, err
}

func existingParent(path string) string {
	for {
		if _, err := os.Stat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

func formatSize(bytes uint64) string {
	if bytes < 1<<30 {
		return fmt.Sprintf("%d MB", (bytes+(1<<20)-1)>>20)
	}
	return strings.Replace(fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30)), ".", ",", 1)
}
