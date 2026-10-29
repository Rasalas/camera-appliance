package cameraaccess

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
)

func captureFrame(ctx context.Context, rawURL, sshHost string) ([]byte, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-rtsp_transport", "tcp", "-i", rawURL, "-frames:v", "1", "-f", "image2", "-vcodec", "mjpeg", "pipe:1"}
	if sshHost != "" {
		if _, err := exec.LookPath("ssh"); err != nil {
			return nil, errors.New("SSH ist nicht installiert; Remote-Capture kann nicht ausgeführt werden")
		}
		sshArgs := append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=3", sshHost, "ffmpeg"}, args...)
		return exec.CommandContext(ctx, "ssh", sshArgs...).Output()
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg ist nicht installiert; Vorschaubild kann nicht erzeugt werden")
	}
	return exec.CommandContext(ctx, ffmpeg, args...).Output()
}

// restreamCaptureTimeout leaves most of the frame budget for the direct fallback.
const restreamCaptureTimeout = 4 * time.Second

// captureGo2RTCFrame fetches a JPEG from go2rtc, which reuses its running camera session.
func captureGo2RTCFrame(ctx context.Context, frameURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, frameURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go2rtc frame: %s", resp.Status)
	}
	image, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	// go2rtc answers 200 with an empty body when it cannot attach to the stream.
	if len(image) < 2 || image[0] != 0xFF || image[1] != 0xD8 {
		return nil, errors.New("go2rtc frame: response is not a JPEG image")
	}
	return image, nil
}

// restreamFrame captures through go2rtc when the request uses the stored credentials.
// New credentials are being tested, so they must reach the camera directly.
func (s *Service) restreamFrame(ctx context.Context, device state.Device, req FrameInput) ([]byte, string, bool) {
	if s.Restream == nil || s.CaptureRestream == nil || strings.TrimSpace(req.Password) != "" {
		return nil, "", false
	}
	settings, _ := s.store.Settings(ctx)
	if username := strings.TrimSpace(req.Username); username != "" && username != settings["camera.credentials."+device.ID+".username"] {
		return nil, "", false
	}
	stream := strings.TrimSpace(req.Stream)
	if stream == "" {
		stream = strings.TrimSpace(settings["camera.credentials."+device.ID+".stream"])
	}
	if stream == "" {
		stream = "stream2"
	}
	frameURL, ok := s.Restream(ctx, device, stream)
	if !ok {
		return nil, "", false
	}
	captureCtx, cancel := context.WithTimeout(ctx, restreamCaptureTimeout)
	defer cancel()
	image, err := s.CaptureRestream(captureCtx, frameURL)
	if err != nil || len(image) == 0 {
		return nil, "", false
	}
	return image, frameURL, true
}

func cameraRTSPURL(username, password, host, port, stream string) string {
	u := neturl.URL{
		Scheme: "rtsp",
		User:   neturl.UserPassword(username, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + strings.TrimLeft(stream, "/"),
	}
	return u.String()
}

func frameCaptureError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("Vorschaubild konnte nicht gezogen werden: Zeitlimit nach 8 Sekunden. Kamera antwortet zu langsam, Stream ist blockiert oder RTSP ist nicht freigegeben.")
	}
	message := "Vorschaubild konnte nicht gezogen werden. Prüfe Benutzername, Passwort, Stream und RTSP-Freigabe."
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		detail := redaction.Text(string(exitErr.Stderr))
		if detail != "" {
			message += " ffmpeg: " + truncate(detail, 360)
		}
	}
	return errors.New(message)
}

func truncate(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	// Cut at a rune boundary so multi-byte characters stay intact.
	cut := value[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

type FrameInput struct {
	CredentialsInput
	Save bool `json:"save"`
}

type Frame struct {
	CapturedAt       time.Time `json:"captured_at"`
	ContentType      string    `json:"content_type"`
	ImageBase64      string    `json:"image_base64"`
	SHA256           string    `json:"sha256"`
	URLRedacted      string    `json:"url_redacted"`
	SavedPath        string    `json:"saved_path"`
	CredentialSource string    `json:"credential_source"`
	IdentityID       string    `json:"identity_id"`
}

func (s *Service) Frame(ctx context.Context, deviceID string, req FrameInput) (Frame, error) {
	device, err := s.store.Device(ctx, deviceID)
	if err != nil {
		return Frame{}, failure(NotFound, err)
	}
	candidates, err := s.frameCredentialCandidates(ctx, device, req.Username, req.Password, req.Stream)
	if err != nil {
		return Frame{}, failure(InvalidInput, err)
	}
	requestCtx := ctx
	if image, frameURL, ok := s.restreamFrame(requestCtx, device, req); ok {
		return s.finishFrame(requestCtx, device, req, image, time.Now(), frameURL, credentialCandidate{Source: "go2rtc"})
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	settings, _ := s.store.Settings(requestCtx)
	captureHost := strings.TrimSpace(settings["capture_ssh_host"])
	if captureHost == "" {
		captureHost = strings.TrimSpace(s.config.CaptureSSHHost)
	}
	endpoint, err := s.endpoint(ctx, device)
	if err != nil {
		return Frame{}, err
	}
	var image []byte
	var rawURL string
	var used credentialCandidate
	var capturedAt time.Time
	var failures []string
	for _, candidate := range candidates {
		rawURL = cameraRTSPURL(candidate.Username, candidate.Password, endpoint.Host, endpoint.Port, candidate.Stream)
		image, err = s.Capture(ctx, rawURL, captureHost)
		if err == nil {
			capturedAt = time.Now()
			used = candidate
			break
		}
		failures = append(failures, candidate.Source+": "+frameCaptureError(ctx, err).Error())
	}
	if len(image) == 0 {
		if len(failures) > 0 {
			return Frame{}, failure(CaptureFailed, errors.New(strings.Join(failures, " · ")))
		}
		return Frame{}, failure(InvalidInput, errors.New("username and password are required for frame capture"))
	}
	return s.finishFrame(requestCtx, device, req, image, capturedAt, rawURL, used)
}

func (s *Service) finishFrame(requestCtx context.Context, device state.Device, req FrameInput, image []byte, capturedAt time.Time, rawURL string, used credentialCandidate) (Frame, error) {
	sum := sha256.Sum256(image)
	imagePath := ""
	if req.Save {
		if err := os.MkdirAll(s.config.ReferenceImageDir(), 0o750); err != nil {
			return Frame{}, err
		}
		imagePath = filepath.Join(s.config.ReferenceImageDir(), device.ID+".jpg")
		if err := os.WriteFile(imagePath, image, 0o600); err != nil {
			return Frame{}, err
		}
		_ = s.store.PutSettings(requestCtx, map[string]string{"camera.reference_image." + device.ID: imagePath, "camera.reference_hash." + device.ID: hex.EncodeToString(sum[:])})
	}
	if used.IdentityID != "" {
		if err := s.rememberIdentityForDevice(requestCtx, device.ID, used); err != nil {
			return Frame{}, err
		}
	}
	return Frame{CapturedAt: capturedAt, ContentType: "image/jpeg", ImageBase64: base64.StdEncoding.EncodeToString(image), SHA256: hex.EncodeToString(sum[:]), URLRedacted: redaction.URL(rawURL), SavedPath: imagePath, CredentialSource: used.Source, IdentityID: used.IdentityID}, nil
}

func (s *Service) ReferenceImage(ctx context.Context, deviceID string) (string, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return "", err
	}
	imagePath := settings["camera.reference_image."+deviceID]
	if imagePath == "" {
		return "", failure(NotFound, errors.New("reference image not found"))
	}
	cleanPath := filepath.Clean(imagePath)
	referenceDir := filepath.Clean(s.config.ReferenceImageDir())
	if cleanPath != filepath.Join(referenceDir, filepath.Base(cleanPath)) {
		return "", failure(InvalidInput, errors.New("invalid reference image path"))
	}
	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		return "", failure(NotFound, errors.New("reference image not found"))
	}
	return cleanPath, nil
}
