package snapshotupload

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/state"
)

func privateSettings() ImageSettings {
	return ImageSettings{Masks: []Mask{{ID: "mask", Mode: "black", X: 10, Y: 10, Width: 30, Height: 30}}, Timestamp: true}
}

func TestImageSettingsPersistPerCameraAndRejectInvalidMasks(t *testing.T) {
	s, db := testService(t)
	ctx := context.Background()
	want := privateSettings()
	if _, err := s.SaveImageSettings(ctx, "device", want); err != nil {
		t.Fatal(err)
	}
	got, err := New(db, s.configDir, s.capture).GetImageSettings(ctx, "device")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("settings not persisted: %+v %v", got, err)
	}
	if err := db.UpsertDevice(ctx, state.Device{ID: "other"}); err != nil {
		t.Fatal(err)
	}
	other, err := s.GetImageSettings(ctx, "other")
	if err != nil || len(other.Masks) != 0 || other.Timestamp {
		t.Fatal("privacy settings leaked across cameras")
	}
	for _, edit := range []func(*ImageSettings){func(c *ImageSettings) { c.Masks = nil }, func(c *ImageSettings) { c.Masks[0].Width = 0 }, func(c *ImageSettings) { c.Masks[0].X = 99 }, func(c *ImageSettings) { c.Masks[0].Y = math.NaN() }, func(c *ImageSettings) { c.Masks[0].Mode = "blur" }, func(c *ImageSettings) { c.Masks = append(c.Masks, c.Masks[0]) }, func(c *ImageSettings) { c.Masks = make([]Mask, 17) }} {
		bad := privateSettings()
		edit(&bad)
		if _, err := s.SaveImageSettings(ctx, "device", bad); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid privacy configuration accepted")
		}
		got, _ = s.GetImageSettings(ctx, "device")
		if !reflect.DeepEqual(got, want) {
			t.Fatal("failed save changed existing protection")
		}
	}
	if _, err := s.SaveImageSettings(ctx, "device", ImageSettings{Masks: []Mask{}}); err != nil {
		t.Fatal("explicit mask removal failed")
	}
}

func TestCorruptPrivacyBlocksManualAndAutomaticUploadsBeforeCapture(t *testing.T) {
	for _, raw := range []string{"", `null`, `{}`, `{"masks":null}`, `{"masks":[]`, `{"masks":[],"unknown":true}`, `{"masks":[{"id":"x","mode":"black","width":200,"height":50}]}`} {
		s, now, calls := scheduledService(t)
		ctx := context.Background()
		captures := 0
		s.capture = func(context.Context, string, cameraaccess.CredentialsInput) (cameraaccess.Frame, error) {
			captures++
			return cameraaccess.Frame{}, nil
		}
		enable(t, s, 60, QuietHours{})
		if err := s.store.PutSettings(ctx, map[string]string{"snapshot.image.device": raw}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Upload(ctx, "device", UploadInput{Crop: &Crop{}}); err == nil {
			t.Fatalf("manual accepted corrupt settings %q", raw)
		}
		*now = now.Add(time.Minute)
		if err := s.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
		status, _ := s.GetSchedule(ctx, "device")
		if status.LastError == "" || status.LastSuccess != nil || captures != 0 || *calls != 0 {
			t.Fatal("corrupt privacy reached capture/transfer or reported success")
		}
	}
}

func TestImageSettingsAcknowledgementWaitsForCurrentUpload(t *testing.T) {
	s, _, _ := scheduledService(t)
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	original := s.capture
	s.capture = func(ctx context.Context, id string, input cameraaccess.CredentialsInput) (cameraaccess.Frame, error) {
		close(started)
		<-release
		return original(ctx, id, input)
	}
	uploaded := make(chan error, 1)
	go func() { _, err := s.Upload(ctx, "device", UploadInput{Crop: &Crop{}}); uploaded <- err }()
	<-started
	saved := make(chan error, 1)
	go func() { _, err := s.SaveImageSettings(ctx, "device", privateSettings()); saved <- err }()
	select {
	case <-saved:
		t.Fatal("new mask set was acknowledged while an old image could still be sent")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-uploaded; err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
}

func TestProcessedImagesReachFTPAndSFTPForManualAndScheduledUploads(t *testing.T) {
	for _, protocol := range []string{"ftp", "sftp"} {
		for _, mode := range []string{"fixed", "unique"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				s, now, _ := scheduledService(t)
				ctx := context.Background()
				data := patternedJPEG(t)
				var cfg Config
				var files <-chan map[string][]byte
				var dir string
				if protocol == "ftp" {
					cfg, files = startFTPServer(t, "", 2, nil)
				} else {
					cfg, dir, _ = startSFTPServer(t, 2, "")
				}
				if _, err := s.SaveSettings(ctx, SettingsInput{Config: cfg, Password: "local-test-password"}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.SaveNaming(ctx, "device", Naming{Mode: mode, Filename: "private.jpg", Directory: cfg.Directory}); err != nil {
					t.Fatal(err)
				}
				settings := privateSettings()
				settings.Masks = append(settings.Masks, Mask{ID: "pixels", Mode: "pixelate", X: 60, Y: 10, Width: 35, Height: 50})
				if _, err := s.SaveImageSettings(ctx, "device", settings); err != nil {
					t.Fatal(err)
				}
				captureAt := time.Date(2026, 9, 5, 12, 34, 56, 0, time.FixedZone("CEST", 7200))
				s.capture = func(context.Context, string, cameraaccess.CredentialsInput) (cameraaccess.Frame, error) {
					return cameraaccess.Frame{ImageBase64: base64.StdEncoding.EncodeToString(data), CapturedAt: captureAt}, nil
				}
				crop := Crop{}
				expected := map[string][]byte{}
				sent := 0
				s.Send = func(ctx context.Context, c Config, p, name string, jpegData []byte) error {
					want, _, _, err := prepareUploadImage(data, crop, settings, captureAt)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(jpegData, want) {
						t.Fatal("transfer did not use saved masks and the fresh capture's local time")
					}
					expected[name] = want
					sent++
					return transfer(ctx, c, p, name, jpegData)
				}
				if _, err := s.Upload(ctx, "device", UploadInput{Crop: &crop}); err != nil {
					t.Fatal(err)
				}
				crop = Crop{Enabled: true, X: 20, Width: 60, Height: 80}
				if err := s.SaveCrop(ctx, "device", crop); err != nil {
					t.Fatal(err)
				}
				enable(t, s, 60, QuietHours{Enabled: true, Start: "22:00", End: "07:00"})
				*now = now.Add(time.Minute)
				captureAt = captureAt.Add(time.Minute)
				if err := s.RunDue(ctx); err != nil {
					t.Fatal(err)
				}
				status, _ := s.GetSchedule(ctx, "device")
				if sent != 2 || status.LastError != "" || status.LastSuccess == nil {
					t.Fatalf("scheduled protected upload failed: %+v", status)
				}
				actual := map[string][]byte{}
				if protocol == "ftp" {
					select {
					case actual = <-files:
					case <-time.After(6 * time.Second):
						t.Fatal("FTP fixture did not finish")
					}
				} else {
					entries, _ := os.ReadDir(dir)
					for _, entry := range entries {
						value, err := os.ReadFile(filepath.Join(dir, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						actual[entry.Name()] = value
					}
				}
				if len(actual) != len(expected) {
					t.Fatal("unexpected remote files")
				}
				for name, want := range expected {
					if !bytes.Equal(actual[name], want) {
						t.Fatal("published JPEG differs from protected result")
					}
				}
			})
		}
	}
}
