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
	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
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
	root.AddCommand(serveCmd(), statusCmd(), discoverCmd(), assignCmd(), renderCmd(), restartGo2RTCCmd(), restartStackCmd(), resetBindingsCmd(), backupCmd(), restoreCmd(), supportBundleCmd())
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

func printViewer(viewer app.Viewer) {
	fmt.Println()
	fmt.Println("Viewer")
	for _, slot := range viewer.Slots {
		fmt.Printf("  %s %s: %s", slot.Alias, slot.Label, viewerStateText(slot.State))
		if slot.Message != "" {
			fmt.Printf(" (%s)", slot.Message)
		}
		fmt.Println()
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

func printJSON(value any) {
	data, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(redaction.Text(string(data)))
}
