package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultRepoAPI  = "https://api.github.com/repos/Rasalas/camera-appliance"
	MaxArchiveBytes = 512 * 1024 * 1024
)

// Release describes one GitHub release with its archive asset.
type Release struct {
	Tag         string `json:"tag_name"`
	Name        string `json:"name"`
	Notes       string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// ArchiveURL picks the versioned archive asset of the release, falling back
// to the rolling latest asset.
func (r Release) ArchiveURL() string {
	for _, asset := range r.Assets {
		if strings.HasSuffix(asset.Name, ".tar.gz") && strings.Contains(asset.Name, "camera-appliance") &&
			!strings.EqualFold(asset.Name, "camera-appliance-latest.tar.gz") {
			return asset.BrowserDownloadURL
		}
	}
	for _, asset := range r.Assets {
		if strings.EqualFold(asset.Name, "camera-appliance-latest.tar.gz") {
			return asset.BrowserDownloadURL
		}
	}
	return DefaultReleaseURL
}

// CompareVersions orders dotted release versions. It returns -1 when a < b,
// 0 when equal and 1 when a > b. A missing part counts as 0 ("1.0" == "1.0.0").
// Versions that are not dotted numbers at all (e.g. "dev", "nas", local build
// labels) always compare as OLDER, so they never mask available updates.
func CompareVersions(a, b string) int {
	a, b = strings.TrimPrefix(strings.TrimSpace(a), "v"), strings.TrimPrefix(strings.TrimSpace(b), "v")
	if a == b {
		return 0
	}
	if !isNumericVersion(a) {
		return -1
	}
	if !isNumericVersion(b) {
		return 1
	}
	aParts, bParts := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aParts) || i < len(bParts); i++ {
		// Missing parts count as 0 so "1.0" equals "1.0.0".
		var as, bs string
		if i < len(aParts) {
			as = aParts[i]
		} else if i < len(bParts) {
			as = "0"
		}
		if i < len(bParts) {
			bs = bParts[i]
		} else if i < len(aParts) {
			bs = "0"
		}
		if as == bs {
			continue
		}
		an, aerr := strconv.Atoi(as)
		bn, berr := strconv.Atoi(bs)
		switch {
		case aerr == nil && berr == nil:
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		default:
			// Mixed shapes (e.g. "1" vs "1-beta"): lexical tiebreak.
			if as < bs {
				return -1
			}
			return 1
		}
	}
	return 0
}

// isNumericVersion reports whether value is built purely from numeric dot
// separated parts ("1.2.3"). Anything else cannot be ordered against releases.
func isNumericVersion(value string) bool {
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// progressStep throttles progress callbacks so the status mutex is not hit
// on every chunk of a large archive.
const progressStep = 256 * 1024

// progressReader counts streamed bytes and reports them through fn, at most
// once per progressStep and never after an error.
type progressReader struct {
	r      io.Reader
	total  int64
	done   int64
	lastAt int64
	fn     func(done, total int64)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 && p.fn != nil {
		p.done += int64(n)
		if p.done-p.lastAt >= progressStep {
			p.lastAt = p.done
			p.fn(p.done, p.total)
		}
	}
	return n, err
}

// FetchArchive downloads the release archive to dir and returns the file path
// plus its sha256 hex digest. onProgress, when non-nil, reports downloaded and
// total bytes while the transfer advances; total is 0 when the server sends no
// Content-Length. The caller owns the returned file.
func FetchArchive(ctx context.Context, url, dir string, httpClient *http.Client, onProgress func(done, total int64)) (path, digest string, err error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("release download failed: %s", resp.Status)
	}
	var body io.Reader = resp.Body
	if onProgress != nil {
		body = &progressReader{r: resp.Body, total: resp.ContentLength, fn: onProgress}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", err
	}
	file, err := os.CreateTemp(dir, "release-*.tar.gz")
	if err != nil {
		return "", "", err
	}
	path = file.Name()
	sum := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, sum), io.LimitReader(body, MaxArchiveBytes))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || written >= MaxArchiveBytes {
		_ = os.Remove(path)
		if written >= MaxArchiveBytes {
			return "", "", fmt.Errorf("release archive exceeds %d bytes limit", MaxArchiveBytes)
		}
		return "", "", err
	}
	if onProgress != nil {
		onProgress(written, resp.ContentLength)
	}
	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chmod(path, info.Mode().Perm()&0o600)
	}
	return path, hex.EncodeToString(sum.Sum(nil)), nil
}

// ArchiveBaseName returns the file name portion of an archive path.
func ArchiveBaseName(path string) string { return filepath.Base(path) }
