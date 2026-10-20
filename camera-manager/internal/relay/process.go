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

	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/streamrouting"
)

func (a *Manager) startSSHRelayProcess(ctx context.Context, relay ManagedRelay) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	newCommand := a.newCommand
	if newCommand == nil {
		newCommand = sshCommand
	}
	args, err := sshRelayArgs(relay)
	if err != nil {
		return 0, err
	}
	logPath := a.relayLogPath(relay.ID)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return 0, err
	}
	// An unrotatable log must not keep the relay down.
	_ = rotateRelayLog(logPath)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	var logOffset int64
	if info, err := logFile.Stat(); err == nil {
		logOffset = info.Size()
	}
	cmd, err := newCommand(args)
	if err != nil {
		_ = logFile.Close()
		return 0, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, err
	}
	_ = logFile.Close()
	a.trackChild(relay.ID, cmd, logOffset)
	return cmd.Process.Pid, nil
}

// relayChild is an ssh process started by this manager. Its Wait goroutine
// reaps it, so an exited relay never lingers as a zombie that looks alive.
type relayChild struct {
	pid       int
	startedAt time.Time
	exited    bool
	exitedAt  time.Time
	exitCode  int
	reported  bool
	// done is closed after the exit fields are set.
	done chan struct{}
	// logOffset is where this run's output starts in the relay log.
	logOffset int64
}

func (a *Manager) trackChild(relayID string, cmd *exec.Cmd, logOffset int64) {
	child := &relayChild{pid: cmd.Process.Pid, startedAt: time.Now().UTC(), done: make(chan struct{}), logOffset: logOffset}
	a.childMu.Lock()
	if a.children == nil {
		a.children = map[string]*relayChild{}
	}
	a.children[relayID] = child
	a.childMu.Unlock()
	go func() {
		_ = cmd.Wait()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		a.childMu.Lock()
		child.exited = true
		child.exitedAt = time.Now().UTC()
		child.exitCode = code
		a.childMu.Unlock()
		close(child.done)
	}()
}

// awaitEarlyExit blocks until the grace window after start has passed and
// returns an error if the tracked child exited within it.
func (a *Manager) awaitEarlyExit(ctx context.Context, relayID string, pid int) error {
	child, ok := a.trackedChild(relayID, pid)
	if !ok || a.startGrace <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Until(child.startedAt.Add(a.startGrace)))
	defer timer.Stop()
	select {
	case <-child.done:
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return nil
	}
	if child, ok = a.trackedChild(relayID, pid); !ok {
		return nil
	}
	message := "Relay-Prozess wurde direkt nach dem Start durch ein Signal beendet."
	if child.exitCode >= 0 {
		message = fmt.Sprintf("Relay-Prozess wurde direkt nach dem Start beendet (Exit-Code %d).", child.exitCode)
	}
	if tail := relayLogTail(a.relayLogPath(relayID), child.logOffset); tail != "" {
		message += " Relay-Log: " + tail
	}
	return errors.New(redaction.Text(message))
}

// relayLogTail returns the last lines written after offset, joined on one line.
func relayLogTail(path string, offset int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() <= offset {
		return ""
	}
	offset = max(offset, info.Size()-relayLogTailBytes)
	data := make([]byte, info.Size()-offset)
	n, _ := file.ReadAt(data, offset)
	var lines []string
	for _, line := range strings.Split(string(data[:n]), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > relayLogTailLines {
		lines = lines[len(lines)-relayLogTailLines:]
	}
	return strings.Join(lines, " | ")
}

// trackedChild returns a snapshot of the child started for relayID if it
// matches pid. Relays from a previous manager process are not tracked.
func (a *Manager) trackedChild(relayID string, pid int) (relayChild, bool) {
	a.childMu.Lock()
	defer a.childMu.Unlock()
	child, ok := a.children[relayID]
	if !ok || pid <= 0 || child.pid != pid {
		return relayChild{}, false
	}
	return *child, true
}

func (a *Manager) markChildReported(relayID string, pid int) {
	a.childMu.Lock()
	defer a.childMu.Unlock()
	if child, ok := a.children[relayID]; ok && child.pid == pid {
		child.reported = true
	}
}

func (a *Manager) forgetChild(relayID string) {
	a.childMu.Lock()
	defer a.childMu.Unlock()
	delete(a.children, relayID)
}

func (a *Manager) relayRunning(relayID string, pid int) bool {
	if child, ok := a.trackedChild(relayID, pid); ok {
		return !child.exited
	}
	return processAlive(pid)
}

func childExitMessage(child relayChild) string {
	if child.exitCode < 0 {
		return "Relay-Prozess wurde durch ein Signal beendet."
	}
	return fmt.Sprintf("Relay-Prozess beendet (Exit-Code %d). Details im Relay-Log.", child.exitCode)
}

// rotateRelayLog moves an oversized log to <path>.1, replacing an older copy.
func rotateRelayLog(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() <= relayLogMaxBytes {
		return nil
	}
	return os.Rename(path, path+".1")
}

func sshCommand(args []string) (*exec.Cmd, error) {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, errors.New("SSH ist nicht installiert; installiere openssh-client oder starte den Relay manuell")
	}
	return exec.Command("ssh", args...), nil
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
	if processIsZombie(pid) {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// processIsZombie catches exited relays nobody reaped, e.g. children of a
// previous manager process. Without /proc (macOS dev) it reports false.
func processIsZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	return statIsZombie(data)
}

// statIsZombie parses /proc/<pid>/stat; the state follows the last ')'
// because the command name may itself contain parentheses.
func statIsZombie(stat []byte) bool {
	end := bytes.LastIndexByte(stat, ')')
	if end < 0 || end+2 >= len(stat) {
		return false
	}
	state := stat[end+2]
	return state == 'Z' || state == 'X'
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

func stopProcess(ctx context.Context, pid int, running func() bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !running() {
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
