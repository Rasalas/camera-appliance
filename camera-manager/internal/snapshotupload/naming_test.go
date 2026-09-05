package snapshotupload

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/state"
	"github.com/pkg/sftp"
)

func TestNamingValidationAndPerCameraPersistence(t *testing.T) {
	s, _, _ := scheduledService(t)
	ctx := context.Background()
	got, err := s.GetNaming(ctx, "device")
	if err != nil || got != (Naming{Mode: "unique"}) {
		t.Fatalf("existing camera default: %+v %v", got, err)
	}
	for _, filename := range []string{"", "../hof.jpg", "/hof.jpg", `dir\hof.jpg`, "dir/hof.jpg", "hof.jpg\r\nDELE image.jpg", "hof.png", " hof.jpg", "hof.jpg ", ".jpg", "-hof.jpg", "hof?.jpg", "hof:jpg", strings.Repeat("a", 117) + ".jpg"} {
		if _, err := s.SaveNaming(ctx, "device", Naming{Mode: "fixed", Filename: filename}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid filename %q: %v", filename, err)
		}
	}
	if _, err := s.SaveNaming(ctx, "device", Naming{Mode: "unknown"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted unknown mode")
	}
	for _, filename := range []string{"hof.jpg", "Hof_2-aktuell.JPEG", strings.Repeat("a", 116) + ".jpg"} {
		want := Naming{Mode: "fixed", Filename: filename}
		if _, err := s.SaveNaming(ctx, "device", want); err != nil {
			t.Fatal(err)
		}
		restarted := New(s.store, s.configDir, s.capture)
		got, err := restarted.GetNaming(ctx, "device")
		if err != nil || got != want {
			t.Fatalf("naming lost after restart: %+v %v", got, err)
		}
	}
	if err := s.store.UpsertDevice(ctx, state.Device{ID: "other"}); err != nil {
		t.Fatal(err)
	}
	other, err := s.GetNaming(ctx, "other")
	if err != nil || other.Mode != "unique" || other.Filename != "" {
		t.Fatalf("naming leaked across cameras: %+v %v", other, err)
	}
}

func TestManualAndScheduledCapturesUseSavedNamingAndCrop(t *testing.T) {
	s, now, _ := scheduledService(t)
	ctx := context.Background()
	crop := Crop{Enabled: true, X: 50, Y: 25, Width: 50, Height: 50}
	if err := s.SaveCrop(ctx, "device", crop); err != nil {
		t.Fatal(err)
	}
	var names []string
	s.Send = func(_ context.Context, _ Config, _, filename string, data []byte) error {
		names = append(names, filename)
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 50 || cfg.Height != 40 {
			t.Fatalf("crop changed with naming policy: %+v %v", cfg, err)
		}
		return nil
	}
	enable(t, s, 60, QuietHours{})
	for _, mode := range []string{"fixed", "unique"} {
		if _, err := s.SaveNaming(ctx, "device", Naming{Mode: mode, Filename: "hof.jpg"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Upload(ctx, "device", UploadInput{Crop: &crop}); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(time.Minute)
		if err := s.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(names) != 4 || names[0] != "hof.jpg" || names[1] != "hof.jpg" || names[2] == names[3] || !strings.HasPrefix(names[2], "camera-") || !strings.HasPrefix(names[3], "camera-") {
		t.Fatalf("manual/scheduled filename policy: %v", names)
	}
	status, _ := s.GetSchedule(ctx, "device")
	if status.LastFilename != names[3] {
		t.Fatal("scheduler did not report published filename")
	}
	s.Send = func(context.Context, Config, string, string, []byte) error { return errors.New("replacement denied") }
	if _, err := s.Upload(ctx, "device", UploadInput{Crop: &crop}); !errors.Is(err, ErrRemote) {
		t.Fatal("failed replacement reported manual success")
	}
	*now = now.Add(time.Minute)
	if err := s.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	failed, _ := s.GetSchedule(ctx, "device")
	if failed.LastError == "" || !failed.LastSuccess.Equal(*status.LastSuccess) || failed.LastFilename != status.LastFilename {
		t.Fatal("failed replacement reported automatic success")
	}
}

func TestFTPReplacesExistingImageOnlyAfterCompleteTransfer(t *testing.T) {
	old := []byte("previous image")
	stale := []byte("abandoned partial upload")
	for _, reject := range []string{"", "write", "rename"} {
		t.Run("reject_"+reject, func(t *testing.T) {
			cfg, result := ftpServer(t, reject, map[string][]byte{"hof.jpg": old, "hof.jpg.part": stale})
			data := testJPEG(t)
			err := transfer(context.Background(), cfg, "local-test-password", "hof.jpg", data)
			if (err == nil) != (reject == "") {
				t.Fatalf("unexpected replacement result: %v", err)
			}
			files := <-result
			want := data
			if reject != "" {
				want = old
			}
			if !bytes.Equal(files["hof.jpg"], want) || !bytes.Equal(files["hof.jpg.part"], stale) || len(files) != 2 {
				t.Fatal("replacement lost old image, damaged stale upload or left a temporary file")
			}
		})
	}
}

func TestSFTPReplacesExistingImageAndIgnoresStalePartial(t *testing.T) {
	cfg, dir, _ := sftpServer(t)
	for _, name := range []string{"hof.jpg", "hof.jpg.part"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	data := testJPEG(t)
	if err := transfer(context.Background(), cfg, "local-test-password", "hof.jpg", data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hof.jpg"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("existing image not replaced: %v", err)
	}
	stale, _ := os.ReadFile(filepath.Join(dir, "hof.jpg.part"))
	files, _ := os.ReadDir(dir)
	if string(stale) != "old" || len(files) != 2 {
		t.Fatal("temporary filename collided or was not cleaned up")
	}
}

type denyRename struct{ sftp.FileCmder }

func (h denyRename) Filecmd(r *sftp.Request) error {
	if r.Method == "Rename" {
		return os.ErrPermission
	}
	return h.FileCmder.Filecmd(r)
}
func (h denyRename) PosixRename(*sftp.Request) error { return os.ErrPermission }

func TestSFTPFailedReplacementPreservesExistingFile(t *testing.T) {
	handlers := sftp.InMemHandler()
	handlers.FileCmd = denyRename{handlers.FileCmd}
	w, err := handlers.FilePut.Filewrite(&sftp.Request{Method: "Put", Filepath: "/hof.jpg", Flags: 2 | 8 | 16})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteAt([]byte("old"), 0); err != nil {
		t.Fatal(err)
	}
	if c, ok := w.(io.Closer); ok {
		_ = c.Close()
	}
	cfg, _, _ := sftpServer(t, handlers)
	err = transfer(context.Background(), cfg, "local-test-password", "hof.jpg", testJPEG(t))
	if err == nil || !strings.Contains(err.Error(), "SFTP-Bild konnte nicht fertiggestellt") {
		t.Fatalf("expected rename failure, got %v", err)
	}
	r, err := handlers.FileGet.Fileread(&sftp.Request{Method: "Get", Filepath: "/hof.jpg", Flags: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 3)
	if _, err := r.ReadAt(got, 0); err != nil || string(got) != "old" {
		t.Fatal("failed replacement lost old image")
	}
	lister, err := handlers.FileList.Filelist(&sftp.Request{Method: "List", Filepath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	files := make([]os.FileInfo, 10)
	n, err := lister.ListAt(files, 0)
	if n != 1 || (err != nil && err != io.EOF) {
		t.Fatalf("failed replacement left temporary files: %d %v", n, err)
	}
}

func TestSFTPWithoutPosixExtensionPublishesNewFilesButPreservesExistingOnes(t *testing.T) {
	// These loopback tests are sequential. Restore the library's global server
	// advertisement only after each fixture has finished its connection.
	if err := sftp.SetSFTPExtensions(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sftp.SetSFTPExtensions("hardlink@openssh.com", "posix-rename@openssh.com", "statvfs@openssh.com")
	})
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			handlers := sftp.InMemHandler()
			old := []byte("previous image")
			if existing {
				w, err := handlers.FilePut.Filewrite(&sftp.Request{Method: "Put", Filepath: "/hof.jpg", Flags: 2 | 8})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.WriteAt(old, 0); err != nil {
					t.Fatal(err)
				}
				if c, ok := w.(io.Closer); ok {
					_ = c.Close()
				}
			}
			cfg, _, _ := sftpServer(t, handlers)
			data := testJPEG(t)
			err := transfer(context.Background(), cfg, "local-test-password", "hof.jpg", data)
			if existing && (err == nil || !strings.Contains(err.Error(), "posix-rename")) {
				t.Fatalf("replacement failure not explained: %v", err)
			}
			if !existing && err != nil {
				t.Fatalf("new filename requires no extension: %v", err)
			}
			r, err := handlers.FileGet.Fileread(&sftp.Request{Method: "Get", Filepath: "/hof.jpg", Flags: 1})
			if err != nil {
				t.Fatal(err)
			}
			want := data
			if existing {
				want = old
			}
			got := make([]byte, len(want))
			if _, err := r.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
				t.Fatal("unexpected destination content")
			}
		})
	}
}
