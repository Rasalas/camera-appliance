package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// These trees contain release-owned source/assets, never customer settings or
// state. Overlaying them keeps deleted files that can break the next build,
// including new pages left behind when restoring an older rollback snapshot.
var replacedReleaseTrees = []string{"frontend/src", "frontend/public", "frontend/tests", "frontend/dist", "camera-manager"}

// Replacement trees are written below these install-root directories first.
// They stay on the install filesystem so the swap is a rename.
const (
	stagingNewDir = ".update-new"
	stagingOldDir = ".update-old"
)

func cleanInstallDir(value string) (string, error) {
	if value == "" {
		value = DefaultInstallDir
	}
	clean, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", err
	}
	if clean == string(filepath.Separator) {
		return "", errors.New("refusing to use filesystem root as install dir")
	}
	return clean, nil
}

func snapshotInstall(ctx context.Context, installDir, rollbackDir string) error {
	info, err := os.Stat(installDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("install dir is not a directory: %s", installDir)
	}
	return copyTree(ctx, installDir, rollbackDir, copyOptions{ExcludeGenerated: true})
}

func applyRelease(ctx context.Context, releaseRoot, installDir string) ([]string, error) {
	staged, err := stageTrees(ctx, releaseRoot, installDir)
	if err != nil {
		return nil, err
	}
	defer staged.discard()
	files, err := overlayRelease(ctx, releaseRoot, installDir)
	if err != nil {
		return nil, err
	}
	if err := staged.swap(); err != nil {
		return nil, err
	}
	return append(staged.files, files...), nil
}

// overlayRelease copies files outside the replaced trees; each file is
// replaced atomically.
func overlayRelease(ctx context.Context, source, installDir string) ([]string, error) {
	var files []string
	err := copyTree(ctx, source, installDir, copyOptions{
		ExcludeGenerated: true,
		SkipReleaseTrees: true,
		OnFile: func(path string) {
			files = append(files, path)
		},
	})
	if err != nil {
		return nil, err
	}
	binary := filepath.Join(installDir, "bin", "camera-appliance")
	if pathExists(binary) {
		if err := os.Chmod(binary, 0o755); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func restoreRollback(ctx context.Context, rollbackDir, installDir string) error {
	if rollbackDir == "" {
		return errors.New("rollback dir is empty")
	}
	if !pathExists(rollbackDir) {
		return fmt.Errorf("rollback dir not found: %s", rollbackDir)
	}
	staged, err := stageTrees(ctx, rollbackDir, installDir)
	if err != nil {
		return err
	}
	defer staged.discard()
	if _, err := overlayRelease(ctx, rollbackDir, installDir); err != nil {
		return err
	}
	return staged.swap()
}

type stagedTrees struct {
	installDir string
	pending    []string
	files      []string
}

// stageTrees writes every release tree present in source below stagingNewDir.
// A failure here leaves the live install untouched.
func stageTrees(ctx context.Context, source, installDir string) (*stagedTrees, error) {
	if err := recoverStaging(installDir); err != nil {
		return nil, err
	}
	s := &stagedTrees{installDir: installDir}
	for _, rel := range replacedReleaseTrees {
		if err := s.stage(ctx, source, rel); err != nil {
			s.discard()
			return nil, err
		}
	}
	return s, nil
}

func (s *stagedTrees) stage(ctx context.Context, source, rel string) error {
	src := filepath.Join(source, filepath.FromSlash(rel))
	info, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("release tree is not a directory: %s", rel)
	}
	dst := filepath.Join(s.installDir, stagingNewDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	err = copyTree(ctx, src, dst, copyOptions{ExcludeGenerated: true, OnFile: func(path string) {
		s.files = append(s.files, rel+"/"+path)
	}})
	if err != nil {
		return err
	}
	s.pending = append(s.pending, rel)
	return nil
}

// swap moves staged trees into place, except those listed in keep.
func (s *stagedTrees) swap(keep ...string) error {
	var rest []string
	for i, rel := range s.pending {
		if slices.Contains(keep, rel) {
			rest = append(rest, rel)
			continue
		}
		if err := swapTree(s.installDir, rel); err != nil {
			s.pending = append(rest, s.pending[i:]...)
			return err
		}
	}
	s.pending = rest
	// The rollback snapshot holds the old trees; free their space now.
	return os.RemoveAll(filepath.Join(s.installDir, stagingOldDir))
}

func (s *stagedTrees) discard() {
	_ = recoverStaging(s.installDir)
	s.pending = nil
}

// swapTree renames the live tree aside before moving the staged one in, so a
// failed second rename can be undone.
func swapTree(installDir, rel string) error {
	live := filepath.Join(installDir, filepath.FromSlash(rel))
	staged := filepath.Join(installDir, stagingNewDir, filepath.FromSlash(rel))
	old := filepath.Join(installDir, stagingOldDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(old), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(live), 0o755); err != nil {
		return err
	}
	moved := false
	if _, err := os.Lstat(live); err == nil {
		if err := os.Rename(live, old); err != nil {
			return err
		}
		moved = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(staged, live); err != nil {
		if moved {
			_ = os.Rename(old, live)
		}
		return err
	}
	return syncDir(filepath.Dir(live))
}

// recoverStaging puts back a live tree that an interrupted swap left only in
// stagingOldDir, then removes all staging leftovers.
func recoverStaging(installDir string) error {
	for _, rel := range replacedReleaseTrees {
		live := filepath.Join(installDir, filepath.FromSlash(rel))
		old := filepath.Join(installDir, stagingOldDir, filepath.FromSlash(rel))
		if _, err := os.Lstat(live); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		if _, err := os.Lstat(old); err == nil {
			if err := os.Rename(old, live); err != nil {
				return err
			}
		}
	}
	return errors.Join(
		os.RemoveAll(filepath.Join(installDir, stagingNewDir)),
		os.RemoveAll(filepath.Join(installDir, stagingOldDir)),
	)
}

func isReleaseTree(rel string) bool {
	return slices.Contains(replacedReleaseTrees, filepath.ToSlash(rel))
}

type copyOptions struct {
	ExcludeGenerated bool
	SkipReleaseTrees bool
	OnFile           func(string)
}

func copyTree(ctx context.Context, src, dst string, opts copyOptions) error {
	dirs := map[string]bool{}
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		if opts.ExcludeGenerated && shouldSkipCopyPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if opts.SkipReleaseTrees && entry.IsDir() && isReleaseTree(rel) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		dirs[filepath.Dir(target)] = true
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := writeFileCopy(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		if opts.OnFile != nil {
			opts.OnFile(filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Directory entries must reach the disk too, or power loss can drop files.
	for dir := range dirs {
		if err := syncDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func shouldSkipCopyPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		lower := strings.ToLower(part)
		if lower == ".ds_store" || strings.HasPrefix(lower, "._") || strings.HasPrefix(lower, ".upload-password-") {
			return true
		}
		switch lower {
		case ".git", ".private", "data", "node_modules", ".release", stagingNewDir, stagingOldDir,
			".env", "local.env", "secrets.env", "admin-password.txt", "snapshot-upload-password.json":
			return true
		}
	}
	return false
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := writeFileCopy(src, dst, mode); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dst))
}

// writeFileCopy replaces dst atomically with fsynced contents; the caller
// syncs the parent directory.
func writeFileCopy(src, dst string, mode os.FileMode) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	err = dir.Sync()
	// Some filesystems cannot fsync directories; file data is synced anyway.
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return err
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
