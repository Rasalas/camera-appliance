package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"camera-appliance/camera-manager/internal/streamrouting"
)

func startSSHRelayProcess(ctx context.Context, relay ManagedRelay, logPath string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return 0, errors.New("SSH ist nicht installiert; installiere openssh-client oder starte den Relay manuell")
	}
	args, err := sshRelayArgs(relay)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	_ = logFile.Close()
	return pid, nil
}

func sshRelayArgs(relay ManagedRelay) ([]string, error) {
	if relay.SSHTarget == "" {
		return nil, errors.New("SSH-Ziel fehlt")
	}
	args := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
	}
	seenLocalPorts := map[string]bool{}
	for _, endpoint := range relay.Endpoints {
		// Incomplete endpoints (no port, no target IP yet) are skipped so one
		// unconfigured camera never blocks the tunnel for the others.
		if strings.TrimSpace(endpoint.LocalPort) == "" || endpoint.TargetHost == "" {
			continue
		}
		if err := validatePort("lokaler Relay-Port", endpoint.LocalPort); err != nil {
			return nil, err
		}
		if err := validatePort("Ziel-Port", endpoint.TargetPort); err != nil {
			return nil, err
		}
		bindHost := streamrouting.RelayBindHost(endpoint.BindHost)
		local := bindHost + ":" + endpoint.LocalPort
		if seenLocalPorts[local] {
			return nil, fmt.Errorf("lokaler Relay-Port doppelt belegt: %s", local)
		}
		seenLocalPorts[local] = true
		args = append(args, "-L", local+":"+endpoint.TargetHost+":"+endpoint.TargetPort)
	}
	if len(seenLocalPorts) == 0 {
		return nil, errors.New("keine Relay-Ports konfiguriert")
	}
	args = append(args, relay.SSHTarget)
	return args, nil
}

func validatePort(label, raw string) error {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s ist ungültig: %q", label, raw)
	}
	return nil
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("ungültige PID-Datei %s", path)
	}
	return pid, nil
}

func writePID(path string, pid int) error {
	if pid <= 0 {
		return errors.New("ungültige Relay-PID")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// processLooksLikeSSH verifies via /proc that the PID still belongs to an ssh
// process before we send it signals. On systems without /proc (macOS dev) the
// check cannot run and we keep the legacy behavior.
func processLooksLikeSSH(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return true
	}
	return bytes.Contains(data, []byte("ssh"))
}

func stopProcess(ctx context.Context, pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		if !waitForExit(ctx, 100*time.Millisecond) {
			return ctx.Err()
		}
	}
	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func waitForExit(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
