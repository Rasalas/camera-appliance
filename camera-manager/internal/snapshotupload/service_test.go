package snapshotupload

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

func testJPEG(t *testing.T) []byte {
	t.Helper()
	i := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			c := color.RGBA{R: 240, A: 255}
			if x >= 50 {
				c = color.RGBA{B: 240, A: 255}
			}
			i.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, i, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCropActualImage(t *testing.T) {
	data := testJPEG(t)
	full, w, h, err := prepareImage(data, Crop{})
	if err != nil || w != 100 || h != 80 || !bytes.Equal(full, data) {
		t.Fatalf("full image changed: %d %d %v", w, h, err)
	}
	part, w, h, err := prepareImage(data, Crop{Enabled: true, X: 50, Y: 25, Width: 50, Height: 50})
	if err != nil || w != 50 || h != 40 {
		t.Fatalf("crop dimensions: %d %d %v", w, h, err)
	}
	img, err := jpeg.Decode(bytes.NewReader(part))
	if err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := img.At(25, 20).RGBA()
	if b < 50000 || r > 10000 {
		t.Fatal("crop did not select the blue right half")
	}
	for _, c := range []Crop{{Enabled: true, Width: 0, Height: 50}, {Enabled: true, X: 90, Width: 20, Height: 50}, {Enabled: true, X: -1, Width: 50, Height: 50}, {Enabled: true, Width: math.NaN(), Height: 50}} {
		if _, _, _, err := prepareImage(data, c); err == nil {
			t.Fatalf("accepted invalid crop %+v", c)
		}
	}
	if _, _, _, err := prepareImage([]byte("not an image"), Crop{}); err == nil {
		t.Fatal("accepted invalid JPEG")
	}
}

func testService(t *testing.T) (*Service, *state.Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := state.Open(context.Background(), filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.UpsertDevice(context.Background(), state.Device{ID: "device", LastIP: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	data := testJPEG(t)
	s := New(db, dir, func(context.Context, string, cameraaccess.CredentialsInput) (cameraaccess.Frame, error) {
		return cameraaccess.Frame{ImageBase64: base64.StdEncoding.EncodeToString(data)}, nil
	})
	return s, db
}

func testConfig() Config {
	return Config{Protocol: "ftp", Host: "localhost", Port: 21, Username: "test-user", Directory: "/images"}
}

func TestSettingsProtectSecretsAndBindPasswordToDestination(t *testing.T) {
	s, db := testService(t)
	ctx := context.Background()
	input := SettingsInput{Config: testConfig(), Password: "  special ' $ password  "}
	got, err := s.SaveSettings(ctx, input)
	if err != nil || !got.PasswordSet {
		t.Fatalf("save: %+v %v", got, err)
	}
	settings, _ := db.Settings(ctx)
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), input.Password) || strings.Contains(settings[configKey], input.Password) {
		t.Fatal("password leaked through public settings")
	}
	p, err := secrets.LoadUpload(s.configDir, input.Config.target())
	if err != nil || p != input.Password {
		t.Fatalf("password not preserved: %v", err)
	}
	info, _ := os.Stat(filepath.Join(s.configDir, secrets.UploadPasswordFile))
	if info.Mode().Perm() != 0o600 {
		t.Fatal("password file is not private")
	}
	input.Password = ""
	input.Directory = "/other"
	if _, err := s.SaveSettings(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.Host = "different.example"
	if _, err := s.SaveSettings(ctx, input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reused password for different host: %v", err)
	}
	input.Host = "localhost"
	input.ClearPassword = true
	if got, err := s.SaveSettings(ctx, input); err != nil || got.PasswordSet {
		t.Fatalf("clear password failed: %v", err)
	}
	if _, err := s.Upload(ctx, "device", UploadInput{Crop: &Crop{}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("upload after password deletion: %v", err)
	}
}

func TestUploadCapturesFreshAndUsesCurrentCrop(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	if _, err := s.SaveSettings(ctx, SettingsInput{Config: testConfig(), Password: "test-secret"}); err != nil {
		t.Fatal(err)
	}
	crop := Crop{Enabled: true, X: 50, Width: 50, Height: 100}
	if err := s.SaveCrop(ctx, "device", crop); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Crop(ctx, "device"); err != nil || got != crop {
		t.Fatalf("crop persistence: %+v %v", got, err)
	}
	captures, sends := 0, 0
	s.capture = func(_ context.Context, id string, input cameraaccess.CredentialsInput) (cameraaccess.Frame, error) {
		captures++
		if id != "device" || input.Stream != "stream1" {
			t.Fatal("capture did not use selected camera/stream")
		}
		return cameraaccess.Frame{ImageBase64: base64.StdEncoding.EncodeToString(testJPEG(t))}, nil
	}
	names := map[string]bool{}
	s.Send = func(_ context.Context, c Config, password, filename string, data []byte) error {
		sends++
		if password != "test-secret" || c.Host != "localhost" {
			t.Fatal("wrong destination credentials")
		}
		if names[filename] || strings.ContainsAny(filename, "/\\") {
			t.Fatal("unsafe or duplicate filename")
		}
		names[filename] = true
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 50 || cfg.Height != 80 {
			t.Fatalf("wrong uploaded image: %+v %v", cfg, err)
		}
		return nil
	}
	for i := 0; i < 2; i++ {
		if _, err := s.Upload(ctx, "device", UploadInput{CredentialsInput: cameraaccess.CredentialsInput{Stream: "stream1"}, Crop: &crop}); err != nil {
			t.Fatal(err)
		}
	}
	if captures != 2 || sends != 2 {
		t.Fatal("upload reused a stale reference image")
	}
	bad := Crop{Enabled: true, Width: 200, Height: 50}
	if _, err := s.Upload(ctx, "device", UploadInput{Crop: &bad}); !errors.Is(err, ErrInvalid) || captures != 2 {
		t.Fatal("invalid crop captured/uploaded a frame")
	}
	s.busy.Lock()
	if _, err := s.Upload(ctx, "device", UploadInput{Crop: &crop}); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent upload accepted")
	}
	s.busy.Unlock()
	s.capture = func(context.Context, string, cameraaccess.CredentialsInput) (cameraaccess.Frame, error) {
		return cameraaccess.Frame{}, errors.New("camera unavailable")
	}
	if _, err := s.Upload(ctx, "device", UploadInput{Crop: &crop}); err == nil || sends != 2 {
		t.Fatal("capture failure caused upload")
	}
}

func TestConfigRejectsCommandInjectionAndInvalidHostKey(t *testing.T) {
	for _, change := range []func(*Config){func(c *Config) { c.Host = "ftp://user:secret@host" }, func(c *Config) { c.Port = 0 }, func(c *Config) { c.Username = "user\r\nPASS injected" }, func(c *Config) { c.Directory = "/images\nDELE file" }, func(c *Config) { c.Protocol = "sftp"; c.HostKey = "invalid" }} {
		c := testConfig()
		change(&c)
		if c.Validate() == nil {
			t.Fatalf("accepted invalid config %+v", c)
		}
	}
}
