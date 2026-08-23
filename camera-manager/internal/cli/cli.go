package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"camera-appliance/camera-manager/internal/app"
	authn "camera-appliance/camera-manager/internal/auth"
	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
	updater "camera-appliance/camera-manager/internal/update"
	api "camera-appliance/camera-manager/internal/web/api"

	"github.com/spf13/cobra"
)

func Execute() error {
	root := &cobra.Command{
		Use:           "camera-appliance",
		Short:         "Local camera viewing appliance manager",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(serveCmd(), statusCmd(), discoverCmd(), assignCmd(), renderCmd(), restartGo2RTCCmd(), restartStackCmd(), relaysCmd(), adminCmd(), resetBindingsCmd(), backupCmd(), restoreCmd(), supportBundleCmd(), installCmd(), updateCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, redaction.Text(err.Error()))
		return err
	}
	return nil
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start API and admin UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			a, err := app.Open(ctx)
			if err != nil {
				return err
			}
			defer a.Close()
			go a.RunWatchdog(ctx)
			server := &http.Server{Addr: a.Config.BindAddr, Handler: api.New(a).Handler()}
			errCh := make(chan error, 1)
			go func() {
				fmt.Printf("camera-appliance läuft auf http://%s\n", a.Config.BindAddr)
				errCh <- server.ListenAndServe()
			}()
			select {
			case <-ctx.Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return server.Shutdown(shutdownCtx)
			case err := <-errCh:
				if err == http.ErrServerClosed {
					return nil
				}
				return err
			}
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show appliance status",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			status, err := a.Status(cmd.Context())
			if err != nil {
				return err
			}
			printStatus(status)
			viewer, err := a.Viewer(cmd.Context())
			if err != nil {
				fmt.Printf("\nViewer: Diagnose konnte nicht geladen werden (%s)\n", redaction.Text(err.Error()))
				return nil
			}
			printViewer(viewer)
			return nil
		},
	}
}

func discoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Run camera discovery once",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()
			a, err := app.Open(ctx)
			if err != nil {
				return err
			}
			defer a.Close()
			result, err := a.Discover(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("Suche abgeschlossen: %d Gerät(e), %d Netzwerk(e)\n", len(result.Devices), len(result.Subnets))
			for _, subnet := range result.Subnets {
				fmt.Printf("  Netzwerk: %s (%s)\n", subnet.CIDR, subnet.Interface)
			}
			for _, device := range result.Devices {
				fmt.Printf("  %s %s %s\n", device.ID, device.LastIP, strings.TrimSpace(device.Manufacturer+" "+device.Model))
			}
			if len(result.Devices) == 0 {
				fmt.Println("Keine Kameras gefunden. Das ist in Testumgebungen ohne Kameras normal.")
			}
			return nil
		},
	}
}

func assignCmd() *cobra.Command {
	var binding state.Binding
	cmd := &cobra.Command{
		Use:   "assign",
		Short: "Assign a discovered device to a slot",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			if err := a.Assign(cmd.Context(), binding); err != nil {
				return err
			}
			fmt.Printf("%s zu %s zugeordnet\n", binding.DeviceID, binding.SlotID)
			return nil
		},
	}
	cmd.Flags().StringVar(&binding.SlotID, "slot", "", "slot id, e.g. cam1")
	cmd.Flags().StringVar(&binding.DeviceID, "device", "", "device id")
	cmd.Flags().StringVar(&binding.Username, "username", "", "camera username")
	cmd.Flags().StringVar(&binding.Label, "label", "", "display label")
	cmd.Flags().StringVar(&binding.StreamName, "stream", "stream2", "stream1 or stream2")
	_ = cmd.MarkFlagRequired("slot")
	_ = cmd.MarkFlagRequired("device")
	return cmd
}

func renderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render-go2rtc",
		Short: "Render generated go2rtc config",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			result, err := a.RenderGo2RTC(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("go2rtc-Konfiguration geschrieben: %s\n", result.Path)
			fmt.Printf("Streams: %d\n", result.RenderedStreams)
			for _, warning := range result.Warnings {
				fmt.Printf("Warnung: %s\n", warning)
			}
			fmt.Print(result.RedactedYAML)
			return nil
		},
	}
}

func restartGo2RTCCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart-go2rtc",
		Short: "Restart go2rtc through Docker Compose",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			if _, err := a.RenderGo2RTC(cmd.Context()); err != nil {
				return err
			}
			if err := system.RestartGo2RTC(cmd.Context(), a.Config); err != nil {
				return err
			}
			fmt.Println("go2rtc wurde neu gestartet")
			return nil
		},
	}
}

func restartStackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart-stack",
		Short: "Restart go2rtc and camera-appliance containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			if err := system.RestartStack(cmd.Context(), a.Config); err != nil {
				return err
			}
			fmt.Println("Kamera-Stack wurde neu gestartet")
			return nil
		},
	}
}

func relaysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relays",
		Short: "Manage SSH camera relays",
	}
	cmd.AddCommand(relayStatusCmd(), relayActionCmd("start"), relayActionCmd("stop"), relayActionCmd("restart"), relayEnsureCmd())
	return cmd
}

func relayStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show relay status",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			statuses, err := a.RelayStatuses(cmd.Context())
			if err != nil {
				return err
			}
			printRelayStatuses(statuses)
			return nil
		},
	}
}

func relayActionCmd(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " RELAY_ID",
		Short: capitalize(action) + " a relay",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			var status app.RelayStatus
			switch action {
			case "start":
				status, err = a.StartRelay(cmd.Context(), args[0])
			case "stop":
				status, err = a.StopRelay(cmd.Context(), args[0])
			case "restart":
				status, err = a.RestartRelay(cmd.Context(), args[0])
			}
			if err != nil {
				return err
			}
			printRelayStatuses([]app.RelayStatus{status})
			return nil
		},
	}
}

func relayEnsureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ensure",
		Short: "Start stopped relays with Auto-Start enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			statuses, err := a.EnsureManagedRelays(cmd.Context())
			printRelayStatuses(statuses)
			return err
		},
	}
}

func adminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage local logins",
	}
	cmd.AddCommand(adminSetPasswordCmd())
	return cmd
}

func adminSetPasswordCmd() *cobra.Command {
	var role string
	var password string
	cmd := &cobra.Command{
		Use:   "set-password",
		Short: "Set the local admin or viewer password",
		RunE: func(cmd *cobra.Command, args []string) error {
			role = strings.ToLower(strings.TrimSpace(role))
			if role == "" {
				role = authn.RoleAdmin
			}
			envName := "CAMERA_APPLIANCE_ADMIN_PASSWORD"
			if role == authn.RoleViewer {
				envName = "CAMERA_APPLIANCE_VIEWER_PASSWORD"
			}
			if password == "" {
				password = os.Getenv(envName)
			}
			if passwordFlagChanged(cmd, "password") {
				fmt.Fprintln(os.Stderr, "WARNUNG: Passwort wurde per --password übergeben und ist in der Shell-History/Prozessliste sichtbar. Bevorzugt: "+envName)
			}
			if strings.TrimSpace(password) == "" {
				return fmt.Errorf("--password oder %s ist erforderlich", envName)
			}
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			if err := a.SetAuthPassword(cmd.Context(), role, password); err != nil {
				return err
			}
			fmt.Printf("Login-Passwort für %s wurde gesetzt\n", role)
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", authn.RoleAdmin, "admin oder viewer")
	cmd.Flags().StringVar(&password, "password", "", "Passwort, alternativ per CAMERA_APPLIANCE_ADMIN_PASSWORD oder CAMERA_APPLIANCE_VIEWER_PASSWORD")
	return cmd
}

func resetBindingsCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset-bindings",
		Short: "Delete discovered devices and camera bindings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("use --yes to confirm deleting bindings and discovered devices")
			}
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			if err := a.ResetBindings(cmd.Context()); err != nil {
				return err
			}
			fmt.Println("Zuordnungen und entdeckte Geräte wurden gelöscht")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm reset")
	return cmd
}

func backupCmd() *cobra.Command {
	var out string
	var includeSecrets bool
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create local configuration backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			result, err := backup.Create(cmd.Context(), a.Config, out, includeSecrets)
			if err != nil {
				return err
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output tar.gz path")
	cmd.Flags().BoolVar(&includeSecrets, "include-secrets", false, "include /etc/camera-appliance/secrets.env")
	return cmd
}

func supportBundleCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "support-bundle",
		Short: "Create a redacted support bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			result, err := a.CreateSupportBundle(cmd.Context(), out)
			if err != nil {
				return err
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output tar.gz path")
	return cmd
}

func insecureUpdateAllowed() bool {
	return os.Getenv("CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE") == "1"
}

func installCmd() *cobra.Command {
	var archivePath string
	var releaseURL string
	var digest string
	var sourceDir string
	var installDir string
	var userName string
	var enableSystemd bool
	var enableKiosk bool
	var installDesktop bool
	var noStart bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the appliance from a release archive",
		RunE: func(cmd *cobra.Command, args []string) error {
			if archivePath == "" && releaseURL == "" && sourceDir == "" {
				releaseURL = updater.DefaultReleaseURL
			}
			result, err := updater.Install(cmd.Context(), updater.InstallOptions{
				Archive:                 archivePath,
				URL:                     releaseURL,
				Digest:                  digest,
				SourceDir:               sourceDir,
				InstallDir:              installDir,
				UserName:                userName,
				EnableSystemd:           enableSystemd,
				EnableKiosk:             enableKiosk,
				InstallDesktopLaunchers: installDesktop,
				NoStart:                 noStart,
				AllowInsecureURL:        insecureUpdateAllowed(),
			})
			printInstallResult(result)
			return err
		},
	}
	cmd.Flags().StringVar(&archivePath, "archive", "", "local release archive .tar.gz")
	cmd.Flags().StringVar(&releaseURL, "url", "", "release archive URL")
	cmd.Flags().StringVar(&digest, "digest", "", "expected sha256 digest of the release archive (sha256:<hex> or bare hex)")
	cmd.Flags().StringVar(&sourceDir, "source-dir", "", "extracted release directory")
	cmd.Flags().StringVar(&installDir, "install-dir", updater.DefaultInstallDir, "installed appliance directory")
	cmd.Flags().StringVar(&userName, "user", os.Getenv("SUDO_USER"), "desktop/kiosk Linux user")
	cmd.Flags().BoolVar(&enableSystemd, "enable-systemd", false, "install and enable camera-appliance.service")
	cmd.Flags().BoolVar(&enableKiosk, "enable-kiosk", false, "install user kiosk service")
	cmd.Flags().BoolVar(&installDesktop, "install-desktop-launchers", false, "install desktop launchers for --user")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "install and enable services without starting them")
	return cmd
}

func updateCmd() *cobra.Command {
	var archivePath string
	var releaseURL string
	var digest string
	var installDir string
	var noRestart bool
	var noAutoRollback bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Apply a release archive with backup and rollback",
		RunE: func(cmd *cobra.Command, args []string) error {
			if archivePath == "" && releaseURL == "" {
				releaseURL = updater.DefaultReleaseURL
			}
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			if installDir == "" {
				installDir = a.Config.InstallDir
			}
			result, err := updater.Apply(cmd.Context(), updater.Options{
				Config:           a.Config,
				Archive:          archivePath,
				URL:              releaseURL,
				Digest:           digest,
				InstallDir:       installDir,
				NoRestart:        noRestart,
				AutoRollback:     !noAutoRollback,
				Restart:          updater.StackRestart(a.Config),
				Healthcheck:      updater.HTTPHealthcheck(a.Config),
				AllowInsecureURL: insecureUpdateAllowed(),
			})
			if result.BackupPath != "" || result.RollbackApplied || len(result.AppliedFiles) > 0 {
				printJSON(result)
			}
			if err != nil {
				_ = a.Store.AddEvent(cmd.Context(), "error", "update.failed", "Update fehlgeschlagen", map[string]any{"rollback_applied": result.RollbackApplied, "rollback_dir": result.RollbackDir})
				return err
			}
			_ = a.Store.AddEvent(cmd.Context(), "info", "update.applied", "Update erfolgreich angewendet", map[string]any{"version": result.NewVersion.Version, "commit": result.NewVersion.Commit, "backup": result.BackupPath})
			return nil
		},
	}
	cmd.Flags().StringVar(&archivePath, "archive", "", "local release archive .tar.gz")
	cmd.Flags().StringVar(&releaseURL, "url", "", "release archive URL")
	cmd.Flags().StringVar(&digest, "digest", "", "expected sha256 digest of the release archive (sha256:<hex> or bare hex)")
	cmd.Flags().StringVar(&installDir, "install-dir", "", "installed appliance directory (default: CAMERA_APPLIANCE_INSTALL_DIR or "+updater.DefaultInstallDir+")")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "copy update without restarting services")
	cmd.Flags().BoolVar(&noAutoRollback, "no-auto-rollback", false, "do not restore previous files when healthcheck fails")
	cmd.AddCommand(updateRollbackCmd())
	return cmd
}

func updateRollbackCmd() *cobra.Command {
	var installDir string
	var noRestart bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Restore the previous update snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			result, err := updater.Rollback(cmd.Context(), updater.RollbackOptions{
				Config:      a.Config,
				InstallDir:  installDir,
				NoRestart:   noRestart,
				Restart:     updater.StackRestart(a.Config),
				Healthcheck: updater.HTTPHealthcheck(a.Config),
			})
			if result.RollbackDir != "" {
				printJSON(result)
			}
			if err != nil {
				_ = a.Store.AddEvent(cmd.Context(), "error", "update.rollback_failed", "Update-Rollback fehlgeschlagen", map[string]any{"rollback_dir": result.RollbackDir})
				return err
			}
			_ = a.Store.AddEvent(cmd.Context(), "warn", "update.rollback", "Update-Rollback ausgeführt", map[string]any{"rollback_dir": result.RollbackDir})
			return nil
		},
	}
	cmd.Flags().StringVar(&installDir, "install-dir", "", "installed appliance directory; defaults to last update")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "restore files without restarting services")
	return cmd
}

func restoreCmd() *cobra.Command {
	var in string
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore local configuration backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer a.Close()
			result, err := backup.Restore(cmd.Context(), a.Config, in)
			if err != nil {
				return err
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "input tar.gz path")
	_ = cmd.MarkFlagRequired("in")
	return cmd
}

func printStatus(status app.Status) {
	fmt.Println("Version")
	versionText := status.Version.Version
	if status.Version.Commit != "" {
		versionText += " (" + status.Version.Commit + ")"
	}
	if status.Version.BuildTime != "" {
		versionText += " " + status.Version.BuildTime
	}
	fmt.Printf("  %s\n\n", versionText)
	fmt.Println("System")
	printService("go2rtc", status.System.Go2RTC)
	printService("camera-appliance", status.System.CameraAppliance)
	printServiceGroup("systemd", status.System.Systemd)
	printServiceGroup("docker compose", status.System.Docker)
	fmt.Println()
	fmt.Println("Zuordnungen")
	if len(status.Bindings) == 0 {
		fmt.Println("  Keine Kameras zugeordnet")
		return
	}
	for _, binding := range status.Bindings {
		label := binding.Label
		if label == "" && binding.Slot != nil {
			label = binding.Slot.Label
		}
		stateText := "keine aktuelle IP"
		ip := ""
		if binding.Device != nil && binding.Device.LastIP != "" {
			stateText = "IP bekannt"
			ip = " " + binding.Device.LastIP
		}
		fmt.Printf("  %s %s: %s%s\n", binding.SlotID, label, stateText, ip)
	}
}

func printInstallResult(result updater.InstallResult) {
	if result.InstallDir == "" {
		return
	}
	fmt.Printf("camera-appliance installiert in %s\n", result.InstallDir)
	fmt.Printf("Version: %s (%s)\n", result.Version.Version, result.Version.Commit)
	if result.SecretsCreated {
		fmt.Println("Secrets: /etc/camera-appliance/secrets.env wurde angelegt. change-me Werte vor Kundeneinsatz ersetzen.")
	} else {
		fmt.Println("Secrets: vorhandene /etc/camera-appliance/secrets.env wurde nicht überschrieben.")
	}
	if result.Go2RTCInitialized {
		fmt.Println("go2rtc: leere Startkonfiguration wurde angelegt.")
	}
	if result.SystemdEnabled {
		if result.Started {
			fmt.Println("Systemd: camera-appliance.service ist aktiviert und gestartet.")
		} else {
			fmt.Println("Systemd: camera-appliance.service ist aktiviert, aber nicht gestartet (--no-start).")
		}
	} else {
		fmt.Println("Systemd: nicht aktiviert.")
	}
	if result.KioskEnabled {
		fmt.Println("Kiosk: Benutzer-Service wurde eingerichtet.")
	}
	if result.DesktopInstalled {
		fmt.Println("Desktop: Starter wurden installiert.")
	}
	fmt.Println()
	fmt.Println("Auf dem Kunden-Laptop öffnen:")
	fmt.Println("  http://127.0.0.1:8091")
	fmt.Println()
	fmt.Println("Firewall-Hinweis:")
	fmt.Println("  UI/API und go2rtc binden standardmäßig nur an localhost/Loopback auf diesem Laptop.")
	fmt.Println("  Für den normalen Kiosk-Betrieb müssen keine eingehenden Ports geöffnet werden.")
	fmt.Println("  Ausgehend braucht der Laptop Zugriff auf Kameras (TCP 554, 2020, 80) sowie HTTPS/Docker-Registry für Installation/Updates.")
	if len(result.Warnings) > 0 {
		fmt.Println()
		fmt.Println("Hinweise:")
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}
}

func printViewer(viewer app.Viewer) {
	fmt.Println()
	fmt.Println("Viewer")
	for _, slot := range viewer.Slots {
		fmt.Printf("  %s %s: %s", slot.Alias, slot.Label, viewerStateText(slot.State))
		if slot.Path != nil {
			fmt.Printf(" via %s %s:%s", slot.Path.ID, slot.Path.Host, slot.Path.Port)
		}
		if slot.Message != "" {
			fmt.Printf(" (%s)", slot.Message)
		}
		fmt.Println()
	}
}

func printRelayStatuses(statuses []app.RelayStatus) {
	fmt.Println("Relays")
	if len(statuses) == 0 {
		fmt.Println("  Keine Relays konfiguriert")
		return
	}
	for _, status := range statuses {
		pid := ""
		if status.PID > 0 {
			pid = fmt.Sprintf(" pid=%d", status.PID)
		}
		fmt.Printf("  %s: %s%s", status.ID, status.ProcessState, pid)
		if status.Message != "" {
			fmt.Printf(" (%s)", status.Message)
		}
		fmt.Println()
		for _, endpoint := range status.Endpoints {
			fmt.Printf("    %s %s:%s -> %s:%s %s", endpoint.DeviceID, endpoint.HealthHost, endpoint.LocalPort, endpoint.TargetHost, endpoint.TargetPort, endpoint.State)
			if endpoint.Message != "" {
				fmt.Printf(" (%s)", endpoint.Message)
			}
			fmt.Println()
		}
	}
}

func viewerStateText(state string) string {
	switch state {
	case app.ViewerStateUnassigned:
		return "leer"
	case app.ViewerStateConnecting:
		return "verbindet"
	case app.ViewerStateOnline:
		return "online"
	case app.ViewerStateOffline:
		return "offline"
	case app.ViewerStateCredentialsFailed:
		return "zugangsdaten fehlen"
	case app.ViewerStateStreamUnavailable:
		return "stream nicht verfügbar"
	default:
		return state
	}
}

func printService(label string, service system.ServiceStatus) {
	state := "offline"
	if service.Online {
		state = "online"
	}
	fmt.Printf("  %s: %s", label, state)
	if service.Message != "" {
		fmt.Printf(" (%s)", service.Message)
	}
	fmt.Println()
}

func printServiceGroup(label string, services []system.ServiceStatus) {
	if len(services) == 0 {
		return
	}
	fmt.Printf("  %s:\n", label)
	for _, service := range services {
		state := "offline"
		if service.Online {
			state = "online"
		}
		fmt.Printf("    %s: %s", service.Name, state)
		if service.Message != "" {
			fmt.Printf(" (%s)", service.Message)
		}
		fmt.Println()
	}
}

func printJSON(value any) {
	data, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(redaction.Text(string(data)))
}

func capitalize(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func passwordFlagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}
