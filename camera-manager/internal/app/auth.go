package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	authn "camera-appliance/camera-manager/internal/auth"
	"camera-appliance/camera-manager/internal/state"
)

const (
	AuthSettingAdminPasswordHash  = "auth.admin.password_hash"
	AuthSettingViewerPasswordHash = "auth.viewer.password_hash"
	AuthSettingViewerPublic       = "auth.viewer_public"
	AuthSettingLocalAdminBypass   = "auth.local_admin_bypass"
	AuthSettingSessionHours       = "auth.session_hours"
)

const defaultSessionHours = 12

type AuthStatus struct {
	Enabled             bool       `json:"enabled"`
	Authenticated       bool       `json:"authenticated"`
	Role                string     `json:"role,omitempty"`
	SessionExpiresAt    *time.Time `json:"session_expires_at,omitempty"`
	AdminPasswordSet    bool       `json:"admin_password_set"`
	ViewerPasswordSet   bool       `json:"viewer_password_set"`
	ViewerPublic        bool       `json:"viewer_public"`
	LocalAdminBypass    bool       `json:"local_admin_bypass"`
	SessionHours        int        `json:"session_hours"`
	LocalAdminBypassNow bool       `json:"local_admin_bypass_now,omitempty"`
}

type LoginResult struct {
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *App) AuthStatus(ctx context.Context, role string, expiresAt *time.Time, localBypass bool) (AuthStatus, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return AuthStatus{}, err
	}
	adminPasswordSet := settings[AuthSettingAdminPasswordHash] != ""
	enabled := adminPasswordSet
	if !enabled {
		role = authn.RoleAdmin
	}
	return AuthStatus{
		Enabled:             enabled,
		Authenticated:       !enabled || authn.IsRole(role),
		Role:                role,
		SessionExpiresAt:    expiresAt,
		AdminPasswordSet:    adminPasswordSet,
		ViewerPasswordSet:   settings[AuthSettingViewerPasswordHash] != "",
		ViewerPublic:        boolSetting(settings, AuthSettingViewerPublic, true),
		LocalAdminBypass:    boolSetting(settings, AuthSettingLocalAdminBypass, false),
		SessionHours:        sessionHours(settings),
		LocalAdminBypassNow: localBypass,
	}, nil
}

func (a *App) SetAuthPassword(ctx context.Context, role, password string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = authn.RoleAdmin
	}
	if !authn.IsRole(role) {
		return fmt.Errorf("unknown auth role %q", role)
	}
	hash, err := authn.HashPassword(password)
	if err != nil {
		return err
	}
	key := AuthSettingAdminPasswordHash
	if role == authn.RoleViewer {
		key = AuthSettingViewerPasswordHash
	}
	if err := a.Store.PutSettings(ctx, map[string]string{key: hash}); err != nil {
		return err
	}
	_ = a.Store.AddEvent(ctx, "info", "auth.password.updated", "Login-Passwort wurde aktualisiert", map[string]string{"role": role})
	return nil
}

func (a *App) Login(ctx context.Context, username, password string) (string, LoginResult, error) {
	role := strings.ToLower(strings.TrimSpace(username))
	if role == "" {
		role = authn.RoleAdmin
	}
	if !authn.IsRole(role) {
		return "", LoginResult{}, errors.New("Benutzername oder Passwort ist falsch")
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return "", LoginResult{}, err
	}
	key := AuthSettingAdminPasswordHash
	if role == authn.RoleViewer {
		key = AuthSettingViewerPasswordHash
	}
	storedHash := settings[key]
	if storedHash == "" || !authn.VerifyPassword(storedHash, password) {
		return "", LoginResult{}, errors.New("Benutzername oder Passwort ist falsch")
	}
	token, err := authn.NewSessionToken()
	if err != nil {
		return "", LoginResult{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(sessionHours(settings)) * time.Hour)
	if err := a.Store.DeleteExpiredAuthSessions(ctx, now); err != nil {
		return "", LoginResult{}, err
	}
	if err := a.Store.SaveAuthSession(ctx, state.AuthSession{
		TokenHash: authn.TokenHash(token),
		Role:      role,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}); err != nil {
		return "", LoginResult{}, err
	}
	_ = a.Store.AddEvent(ctx, "info", "auth.login", "Login erfolgreich", map[string]string{"role": role})
	return token, LoginResult{Role: role, ExpiresAt: expiresAt}, nil
}

func (a *App) AuthSession(ctx context.Context, token string) (state.AuthSession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return state.AuthSession{}, sql.ErrNoRows
	}
	return a.Store.AuthSession(ctx, authn.TokenHash(token), time.Now().UTC())
}

func (a *App) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return a.Store.DeleteAuthSession(ctx, authn.TokenHash(token))
}

func sessionHours(settings map[string]string) int {
	raw := strings.TrimSpace(settings[AuthSettingSessionHours])
	if raw == "" {
		return defaultSessionHours
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > 168 {
		return defaultSessionHours
	}
	return parsed
}
