package releasearchive

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func sha256Line(payload []byte, name string) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]) + "  " + name + "\n"
}

// releaseServer serves payload under /v1/<any>.tar.gz and sums as /v1/SHA256SUMS
// (404 when sums is empty). archiveHits counts archive downloads when non-nil.
func releaseServer(t *testing.T, payload []byte, sums string, archiveHits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/"+ChecksumsFile && sums != "":
			_, _ = w.Write([]byte(sums))
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			if archiveHits != nil {
				archiveHits.Add(1)
			}
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestParseChecksums(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	sums, err := ParseChecksums([]byte(digest + "  camera-appliance-1.0.0-abc.tar.gz\n" + strings.ToUpper(digest) + " *camera-appliance-latest.tar.gz\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sums["camera-appliance-1.0.0-abc.tar.gz"] != digest || sums["camera-appliance-latest.tar.gz"] != digest {
		t.Fatalf("unexpected sums %v", sums)
	}
	for name, input := range map[string]string{
		"short digest":       "abc  release.tar.gz\n",
		"missing name":       digest + "\n",
		"non-hex":            strings.Repeat("zz", 32) + "  release.tar.gz\n",
		"conflicting digest": digest + "  a.tar.gz\n" + strings.Repeat("cd", 32) + "  a.tar.gz\n",
	} {
		if _, err := ParseChecksums([]byte(input)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestPrepareURLRequiresPublishedChecksum(t *testing.T) {
	payload, err := os.ReadFile(testArchive(t, map[string]string{"release/bin/camera-appliance": "binary"}))
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(server *httptest.Server, digest string) error {
		workspace := t.TempDir()
		release, err := Prepare(context.Background(), Source{URL: server.URL + "/v1/camera-appliance-1.0.0-abc.tar.gz", Digest: digest, AllowInsecureURL: true}, workspace, server.Client())
		if release != nil {
			release.Close()
		}
		if entries, _ := os.ReadDir(workspace); len(entries) != 0 {
			t.Fatalf("staging leaked: %v", entries)
		}
		return err
	}

	var hits atomic.Int32
	if err := prepare(releaseServer(t, payload, "", &hits), ""); !errors.Is(err, ErrChecksumsMissing) {
		t.Fatalf("release without SHA256SUMS must be refused, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("archive downloaded although it cannot be verified")
	}
	if err := prepare(releaseServer(t, payload, sha256Line([]byte("other"), "camera-appliance-1.0.0-abc.tar.gz"), nil), ""); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("mismatching archive accepted: %v", err)
	}
	if err := prepare(releaseServer(t, payload, sha256Line(payload, "camera-appliance-latest.tar.gz"), nil), ""); err == nil || !strings.Contains(err.Error(), "keinen Eintrag") {
		t.Fatalf("archive missing from SHA256SUMS accepted: %v", err)
	}
	if err := prepare(releaseServer(t, payload, sha256Line(payload, "camera-appliance-1.0.0-abc.tar.gz"), nil), ""); err != nil {
		t.Fatalf("listed archive rejected: %v", err)
	}
	// An explicit digest is the operator override for releases predating SHA256SUMS.
	sum := sha256.Sum256(payload)
	if err := prepare(releaseServer(t, payload, "", nil), "sha256:"+hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("explicit digest override rejected: %v", err)
	}
}

func elfHeader(class elf.Class, machine elf.Machine, osabi elf.OSABI) []byte {
	size := 64
	if class == elf.ELFCLASS32 {
		size = 52
	}
	header := make([]byte, size)
	copy(header, elf.ELFMAG)
	header[elf.EI_CLASS] = byte(class)
	header[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	header[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	header[elf.EI_OSABI] = byte(osabi)
	le := binary.LittleEndian
	le.PutUint16(header[16:], uint16(elf.ET_EXEC))
	le.PutUint16(header[18:], uint16(machine))
	le.PutUint32(header[20:], uint32(elf.EV_CURRENT))
	if class == elf.ELFCLASS32 {
		le.PutUint16(header[40:], uint16(size))
	} else {
		le.PutUint16(header[52:], uint16(size))
	}
	return header
}

func TestCheckReleaseBinaryMatchesPlatform(t *testing.T) {
	write := func(content []byte) string {
		path := filepath.Join(t.TempDir(), "camera-appliance")
		if err := os.WriteFile(path, content, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	machO := []byte{0xcf, 0xfa, 0xed, 0xfe, 0x07, 0x00, 0x00, 0x01}
	cases := []struct {
		name, goos, goarch string
		content            []byte
		ok                 bool
	}{
		{"linux amd64", "linux", "amd64", elfHeader(elf.ELFCLASS64, elf.EM_X86_64, elf.ELFOSABI_NONE), true},
		{"linux arm64", "linux", "arm64", elfHeader(elf.ELFCLASS64, elf.EM_AARCH64, elf.ELFOSABI_NONE), true},
		{"linux 386", "linux", "386", elfHeader(elf.ELFCLASS32, elf.EM_386, elf.ELFOSABI_LINUX), true},
		{"arm64 on amd64", "linux", "amd64", elfHeader(elf.ELFCLASS64, elf.EM_AARCH64, elf.ELFOSABI_NONE), false},
		{"freebsd on linux", "linux", "amd64", elfHeader(elf.ELFCLASS64, elf.EM_X86_64, elf.ELFOSABI_FREEBSD), false},
		{"macOS on linux", "linux", "amd64", machO, false},
		{"windows on linux", "linux", "amd64", []byte("MZ\x90\x00"), false},
		{"linux on macOS", "darwin", "arm64", elfHeader(elf.ELFCLASS64, elf.EM_AARCH64, elf.ELFOSABI_NONE), false},
		{"truncated ELF", "linux", "amd64", []byte("\x7fELF\x02\x01"), false},
		{"empty", "linux", "amd64", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkReleaseBinary(write(tc.content), tc.goos, tc.goarch)
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v, got %v", tc.ok, err)
			}
			if err != nil && !strings.Contains(err.Error(), "Release-Archiv") {
				t.Fatalf("error must explain the release problem: %v", err)
			}
		})
	}
}

func TestPrepareRejectsReleaseBinaryForAnotherPlatform(t *testing.T) {
	archive := testArchive(t, map[string]string{"release/bin/camera-appliance": "MZ\x90\x00windows"})
	workspace := t.TempDir()
	release, err := Prepare(context.Background(), Source{Archive: archive}, workspace, nil)
	if err == nil || release != nil || !strings.Contains(err.Error(), "passt nicht zu diesem System") {
		t.Fatalf("foreign binary accepted: %v", err)
	}
	if entries, _ := os.ReadDir(workspace); len(entries) != 0 {
		t.Fatalf("failed staging leaked: %v", entries)
	}
}
