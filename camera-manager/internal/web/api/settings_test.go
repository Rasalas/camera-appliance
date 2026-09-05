package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestStaleSettingsCannotRemoveNewIdentityOrResetRuntime(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a := newAuthTestApp(t)
	initial := map[string]string{"camera.identity.ids": "old", "camera.identity.old.name": "Old", "camera.identity.old.username": "old", "watchdog.last_action": "before"}
	if err := a.Store.PutSettings(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	h := New(a).Handler()
	loaded := performJSON(h, http.MethodGet, "/api/settings", nil, nil)
	var snapshot map[string]string
	if err := json.Unmarshal(loaded.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	created := performJSON(h, http.MethodPost, "/api/credential-identities", map[string]string{"id": "new", "name": "New", "username": "new"}, nil)
	if created.Code != http.StatusOK {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	if err := a.Store.PutSettings(context.Background(), map[string]string{"watchdog.last_action": "after"}); err != nil {
		t.Fatal(err)
	}
	snapshot["capture_ssh_host"] = "relay"
	saved := performJSON(h, http.MethodPut, "/api/settings", snapshot, nil)
	if saved.Code != http.StatusOK {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	settings, err := a.Store.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings["camera.identity.ids"] != "old,new" || settings["watchdog.last_action"] != "after" || settings["capture_ssh_host"] != "relay" {
		t.Fatalf("configuration clobbered owned state: %v", settings)
	}
}
