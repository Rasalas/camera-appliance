package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	if pathExists(filepath.Join(releaseRoot, "frontend", "dist")) {
		if err := os.RemoveAll(filepath.Join(installDir, "frontend", "dist")); err != nil {
			return nil, err
		}
	}
	var files []string
	err := copyTree(ctx, releaseRoot, installDir, copyOptions{
		ExcludeGenerated: true,
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
	if pathExists(filepath.Join(rollbackDir, "frontend", "dist")) {
		if err := os.RemoveAll(filepath.Join(installDir, "frontend", "dist")); err != nil {
			return err
		}
	}
	return copyTree(ctx, rollbackDir, installDir, copyOptions{ExcludeGenerated: true})
}

type copyOptions struct {
	ExcludeGenerated bool
	OnFile           func(string)
}

func copyTree(ctx context.Context, src, dst string, opts copyOptions) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
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
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
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
		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		if opts.OnFile != nil {
			opts.OnFile(filepath.ToSlash(rel))
		}
		return nil
	})
}

func shouldSkipCopyPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		lower := strings.ToLower(part)
		if lower == ".ds_store" || strings.HasPrefix(lower, "._") || strings.HasPrefix(lower, ".upload-password-") {
			return true
		}
		switch lower {
		case ".git", ".private", "data", "node_modules", ".release",
			".env", "local.env", "secrets.env", "admin-password.txt", "snapshot-upload-password.json":
			return true
		}
	}
	return false
}

func copyFile(src, dst string, mode os.FileMode) error {
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
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
