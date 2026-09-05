package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCameraPasswordCanChangeWhileViewerReadsConfig(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) }))
	defer server.Close()
	a := newViewerTestApp(t, server.URL, "")
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				a.SetCameraCredentials("test-password", "test")
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := 0; i < 10; i++ {
		if _, err := a.Viewer(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPasswordChangeRevokesRoleSessions(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "")
	for _, role := range []string{"admin", "viewer"} {
		if err := a.SetAuthPassword(ctx, role, "old-password"); err != nil {
			t.Fatal(err)
		}
	}
	admin, _, err := a.Login(ctx, "admin", "old-password", true)
	if err != nil {
		t.Fatal(err)
	}
	viewer, _, err := a.Login(ctx, "viewer", "old-password", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetAuthPassword(ctx, "admin", "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AuthSession(ctx, admin); err == nil {
		t.Error("old admin session survived password change")
	}
	if _, err := a.AuthSession(ctx, viewer); err != nil {
		t.Errorf("unrelated viewer session revoked: %v", err)
	}
	if _, _, err := a.Login(ctx, "admin", "old-password", false); err == nil {
		t.Error("old password accepted")
	}
	if _, _, err := a.Login(ctx, "admin", "new-password", false); err != nil {
		t.Fatal(err)
	}
}
