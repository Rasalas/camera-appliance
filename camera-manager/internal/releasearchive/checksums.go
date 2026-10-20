package releasearchive

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// ChecksumsFile is the release asset listing "<sha256>  <asset>" for every archive.
const ChecksumsFile = "SHA256SUMS"

// ErrChecksumsMissing marks releases published before SHA256SUMS existed.
var ErrChecksumsMissing = errors.New("Release ohne " + ChecksumsFile)

// ParseChecksums reads sha256sum output. Binary-mode markers ("*name") are accepted.
func ParseChecksums(data []byte) (map[string]string, error) {
	sums := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s line %d is malformed", ChecksumsFile, line)
		}
		digest, name := strings.ToLower(fields[0]), strings.TrimPrefix(fields[1], "*")
		if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("%s line %d has no valid sha256 digest", ChecksumsFile, line)
		}
		if previous, ok := sums[name]; ok && previous != digest {
			return nil, fmt.Errorf("%s lists %s twice with different digests", ChecksumsFile, name)
		}
		sums[name] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sums, nil
}

// checksumsLocation returns the sibling SHA256SUMS URL and the asset name it must list.
func checksumsLocation(archiveURL string) (string, string, error) {
	parsed, err := url.Parse(archiveURL)
	if err != nil {
		return "", "", err
	}
	name := path.Base(parsed.Path)
	if parsed.Path == "" || name == "/" || name == "." {
		return "", "", fmt.Errorf("archive URL %q names no file", redactUpdateURL(archiveURL))
	}
	parsed.Path = path.Join(path.Dir(parsed.Path), ChecksumsFile)
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	return parsed.String(), name, nil
}

// ExpectedDigest loads SHA256SUMS published next to archiveURL and returns the
// archive's bare hex digest. Releases without the file yield ErrChecksumsMissing.
func ExpectedDigest(ctx context.Context, client *http.Client, archiveURL string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	sumsURL, name, err := checksumsLocation(archiveURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s konnte nicht geladen werden: %w", ChecksumsFile, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%w: das Archiv kann nicht geprüft werden. Update abgelehnt; ältere Releases nur per Kommandozeile mit --digest sha256:<Prüfsumme> installieren", ErrChecksumsMissing)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s konnte nicht geladen werden: %s", ChecksumsFile, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	sums, err := ParseChecksums(data)
	if err != nil {
		return "", err
	}
	digest, ok := sums[name]
	if !ok {
		return "", fmt.Errorf("%s enthält keinen Eintrag für %s", ChecksumsFile, name)
	}
	return digest, nil
}
