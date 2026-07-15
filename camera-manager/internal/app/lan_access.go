package app

import (
	"context"
	"fmt"
	"net"
)

const NetworkSettingLANAccess = "network.lan_access_enabled"

func (a *App) applyNetworkAccess(ctx context.Context) error {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return err
	}
	enabled := settings[NetworkSettingLANAccess]
	if settings[AuthSettingAdminPasswordHash] == "" {
		enabled = "false"
	}
	a.Config.BindAddr, err = effectiveBindAddr(a.Config.BindAddr, enabled)
	return err
}

func effectiveBindAddr(configured, enabled string) (string, error) {
	if enabled == "" {
		return configured, nil
	}
	_, port, err := net.SplitHostPort(configured)
	if err != nil {
		return "", fmt.Errorf("invalid configured bind address %q: %w", configured, err)
	}
	switch enabled {
	case "true":
		return net.JoinHostPort("0.0.0.0", port), nil
	case "false":
		return net.JoinHostPort("127.0.0.1", port), nil
	default:
		return "", fmt.Errorf("invalid LAN access setting %q", enabled)
	}
}

func LANAccessEnabled(bindAddr string) bool {
	host, _, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "" || host == "0.0.0.0" || host == "::" || (ip != nil && !ip.IsLoopback())
}
