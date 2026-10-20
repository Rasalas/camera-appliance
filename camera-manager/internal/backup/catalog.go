package backup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

// Kind tells how a backup was created. It is encoded in the file name.
type Kind string

const (
	KindManual    Kind = "manual"
	KindAutomatic Kind = "automatic"
	KindUpdate    Kind = "before-update"
)

// Entry describes one backup archive in the backup directory.
type Entry struct {
	Name      string    `json:"name"`
	Kind      Kind      `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

var (
	ErrInvalidName = errors.New("Ungültiger Name der Sicherung.")
	ErrNotFound    = errors.New("Sicherung nicht gefunden.")
)

const timestampLayout = "20060102-150405"

// Legacy and manual backups have no kind segment.
var namePattern = regexp.MustCompile(`^camera-appliance-(?:(auto|update)-)?(\d{8}-\d{6}(?:\.\d{1,9})?)\.tar\.gz$`)

var kindSegments = map[Kind]string{KindManual: "", KindAutomatic: "auto-", KindUpdate: "update-"}

// NewPath returns a fresh archive path in the backup directory for kind.
func NewPath(cfg config.Config, kind Kind) string {
	stamp := time.Now().UTC().Format(timestampLayout + ".000000000")
	return filepath.Join(cfg.BackupDir(), "camera-appliance-"+kindSegments[kind]+stamp+".tar.gz")
}

func parseName(name string) (Entry, bool) {
	match := namePattern.FindStringSubmatch(name)
	if match == nil {
		return Entry{}, false
	}
	created, err := time.ParseInLocation(timestampLayout, match[2], time.UTC)
	if err != nil {
		return Entry{}, false
	}
	kind := KindManual
	switch match[1] {
	case "auto":
		kind = KindAutomatic
	case "update":
		kind = KindUpdate
	}
	return Entry{Name: name, Kind: kind, CreatedAt: created}, true
}

// List returns regular backup files, newest first. Other files and symlinks are skipped.
func List(cfg config.Config) ([]Entry, error) {
	items, err := os.ReadDir(cfg.BackupDir())
	if errors.Is(err, fs.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, item := range items {
		if !item.Type().IsRegular() {
			continue
		}
		entry, ok := parseName(item.Name())
		if !ok {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		entry.Size = info.Size()
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].Name > entries[j].Name
	})
	return entries, nil
}

// Open returns a listed backup for reading. The caller closes the file.
func Open(cfg config.Config, name string) (*os.File, Entry, error) {
	root, entry, err := openEntry(cfg, name)
	if err != nil {
		return nil, Entry{}, err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, Entry{}, notFound(err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, Entry{}, ErrNotFound
	}
	entry.Size = info.Size()
	return file, entry, nil
}

// Delete removes one listed backup.
func Delete(cfg config.Config, name string) error {
	root, _, err := openEntry(cfg, name)
	if err != nil {
		return err
	}
	defer root.Close()
	return notFound(root.Remove(name))
}

// Prune deletes the oldest backups of kind beyond keep and returns their names.
func Prune(cfg config.Config, kind Kind, keep int) ([]string, error) {
	entries, err := List(cfg)
	if err != nil {
		return nil, err
	}
	var removed []string
	var errs []error
	kept := 0
	for _, entry := range entries {
		if entry.Kind != kind {
			continue
		}
		if kept < keep {
			kept++
			continue
		}
		if err := Delete(cfg, entry.Name); err != nil && !errors.Is(err, ErrNotFound) {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, entry.Name)
	}
	return removed, errors.Join(errs...)
}

// openEntry validates name and confines access to the backup directory via os.Root.
func openEntry(cfg config.Config, name string) (*os.Root, Entry, error) {
	entry, ok := parseName(name)
	if !ok {
		return nil, Entry{}, ErrInvalidName
	}
	root, err := os.OpenRoot(cfg.BackupDir())
	if err != nil {
		return nil, Entry{}, notFound(err)
	}
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		root.Close()
		return nil, Entry{}, ErrNotFound
	}
	return root, entry, nil
}

func notFound(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	return err
}
