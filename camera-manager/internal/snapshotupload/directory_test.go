package snapshotupload

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/state"
)

func TestCameraDirectoryValidationNormalizationAndLegacyDefault(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	for _, directory := range []string{"../images", "images/../other", `/images\hof`, "ftp://host/images", "C:/images", "images\n", "images\x00", strings.Repeat("a", 1025)} {
		if _, err := s.SaveNaming(ctx, "device", Naming{Mode: "unique", Directory: directory}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted directory %q: %v", directory, err)
		}
	}
	for input, want := range map[string]string{"": "", "  ": "", " . ": ".", "/": "/", "./bilder//hof/": "bilder/hof", " /bilder/straße hof/ ": "/bilder/straße hof"} {
		got, err := s.SaveNaming(ctx, "device", Naming{Mode: "fixed", Filename: "aktuell.jpg", Directory: input})
		if err != nil || got.Directory != want {
			t.Fatalf("normalize %q: %+v %v", input, got, err)
		}
		restarted := New(s.store, s.configDir, s.capture)
		saved, err := restarted.GetNaming(ctx, "device")
		if err != nil || saved != got {
			t.Fatalf("directory lost after restart: %+v %v", saved, err)
		}
	}
	if err := s.store.PutSettings(ctx, map[string]string{"snapshot.naming.device": `{"mode":"fixed","filename":"existing.jpg"}`}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetNaming(ctx, "device")
	if err != nil || got.Directory != "" || got.Filename != "existing.jpg" {
		t.Fatalf("legacy naming changed: %+v %v", got, err)
	}
}

func TestTwoCamerasTransferIntoSeparateDirectories(t *testing.T) {
	for _, protocol := range []string{"ftp", "sftp"} {
		for _, mode := range []string{"fixed", "unique"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				folders := []string{"cameras/hof", "cameras/garage", "fallback"}
				initial := map[string][]byte{}
				for _, folder := range folders {
					if err := os.MkdirAll(filepath.Join(root, folder), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "fixed" {
					for _, folder := range folders[:2] {
						initial[path.Join("/", folder, "aktuell.jpg")] = []byte("old image")
						if err := os.WriteFile(filepath.Join(root, folder, "aktuell.jpg"), []byte("old image"), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				var cfg Config
				var ftpFiles <-chan map[string][]byte
				directories := []string{"cameras/hof", "/cameras/garage"}
				if protocol == "ftp" {
					cfg, ftpFiles = startFTPServer(t, "", 6, map[string]bool{"/cameras/hof": true, "/cameras/garage": true, "/fallback": true}, initial)
				} else {
					cfg, _, _ = startSFTPServer(t, 6, root)
					directories[1] = filepath.ToSlash(filepath.Join(root, "cameras/garage"))
				}
				cfg.Directory = "fallback"
				s, db := testService(t)
				if err := db.UpsertDevice(ctx, state.Device{ID: "other"}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.SaveSettings(ctx, SettingsInput{Config: cfg, Password: "local-test-password"}); err != nil {
					t.Fatal(err)
				}
				now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
				s.now = func() time.Time { return now }
				ids := []string{"device", "other"}
				crops := []Crop{{Enabled: true, X: 50, Y: 25, Width: 50, Height: 50}, {Enabled: true, X: 0, Y: 25, Width: 50, Height: 50}}
				expected := map[string][]byte{}
				for i, id := range ids {
					if _, err := s.SaveNaming(ctx, id, Naming{Mode: mode, Filename: "aktuell.jpg", Directory: directories[i]}); err != nil {
						t.Fatal(err)
					}
					if err := s.SaveCrop(ctx, id, crops[i]); err != nil {
						t.Fatal(err)
					}
					if _, err := s.SaveSchedule(ctx, id, ScheduleInput{Enabled: true, IntervalSeconds: 60, QuietHours: QuietHours{Enabled: true, Start: "22:00", End: "07:00"}}); err != nil {
						t.Fatal(err)
					}
					// Manual full frames and automatic crops share the same destination.
					result, err := s.Upload(ctx, id, UploadInput{Crop: &Crop{}})
					if err != nil || result.Width != 100 || result.Height != 80 {
						t.Fatalf("manual capture: %+v %v", result, err)
					}
					expected[path.Join(folders[i], result.Filename)] = testJPEG(t)
				}
				now = now.Add(time.Minute)
				if err := s.RunDue(ctx); err != nil {
					t.Fatal(err)
				}
				for i, id := range ids {
					status, err := s.GetSchedule(ctx, id)
					if err != nil || status.LastError != "" || status.LastSuccess == nil {
						t.Fatalf("automatic capture: %+v %v", status, err)
					}
					data, _, _, err := prepareImage(testJPEG(t), crops[i])
					if err != nil {
						t.Fatal(err)
					}
					expected[path.Join(folders[i], status.LastFilename)] = data
				}
				// Clearing an override restores the global destination; it never appends
				// the previous camera directory or changes the shared server settings.
				if _, err := s.SaveNaming(ctx, "device", Naming{Mode: mode, Filename: "aktuell.jpg"}); err != nil {
					t.Fatal(err)
				}
				result, err := s.Upload(ctx, "device", UploadInput{Crop: &Crop{}})
				if err != nil {
					t.Fatal(err)
				}
				expected[path.Join("fallback", result.Filename)] = testJPEG(t)
				if _, err := s.SaveNaming(ctx, "other", Naming{Mode: mode, Filename: "aktuell.jpg", Directory: "missing/subfolder"}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Upload(ctx, "other", UploadInput{Crop: &Crop{}}); !errors.Is(err, ErrRemote) {
					t.Fatalf("missing directory silently fell back: %v", err)
				}
				settings, err := s.Settings(ctx)
				if err != nil || settings.Directory != "fallback" || !settings.PasswordSet {
					t.Fatalf("camera directory changed shared settings: %+v %v", settings, err)
				}
				actual := map[string][]byte{}
				if protocol == "ftp" {
					select {
					case files := <-ftpFiles:
						for name, data := range files {
							actual[strings.TrimPrefix(name, "/")] = data
						}
					case <-time.After(6 * time.Second):
						t.Fatal("FTP fixture did not complete all transfers")
					}
				} else {
					if err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
						if err != nil || d.IsDir() {
							return err
						}
						rel, err := filepath.Rel(root, name)
						if err != nil {
							return err
						}
						data, err := os.ReadFile(name)
						actual[filepath.ToSlash(rel)] = data
						return err
					}); err != nil {
						t.Fatal(err)
					}
				}
				if len(actual) != len(expected) {
					t.Fatalf("unexpected file count: got %d want %d", len(actual), len(expected))
				}
				for name, want := range expected {
					if !bytes.Equal(actual[name], want) {
						t.Fatalf("wrong image or camera directory: %s", name)
					}
				}
			})
		}
	}
}
