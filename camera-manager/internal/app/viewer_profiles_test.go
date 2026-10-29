package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestViewerProfilesAreIndependentAndExposeOnlyLayouts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "secret")
	ctx := context.Background()
	standard := `{"type":"leaf","slot":"cam1"}`
	ipad := `{"type":"leaf","slot":"cam5"}`
	if err := a.UpdateSettings(ctx, map[string]string{
		"viewer.layout.mosaic":            standard,
		"viewer.profiles.ipad.mosaic":     ipad,
		"viewer.profiles.office.mosaic":   standard,
		"viewer.profiles.ipad.password":   "hidden",
		"viewer.profiles.bad.name.mosaic": "hidden",
	}); err != nil {
		t.Fatal(err)
	}
	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Layout.Mosaic != standard || viewer.Profiles["ipad"] != ipad || viewer.Profiles["office"] != standard || len(viewer.Profiles) != 2 {
		t.Fatalf("unexpected profiles: %+v, default: %s", viewer.Profiles, viewer.Layout.Mosaic)
	}
	updated := `{"type":"leaf","slot":"cam3"}`
	if err := a.UpdateSettings(ctx, map[string]string{"viewer.profiles.ipad.mosaic": updated}); err != nil {
		t.Fatal(err)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["viewer.profiles.ipad.mosaic"] != updated || settings["viewer.layout.mosaic"] != standard || settings["viewer.profiles.office.mosaic"] != standard {
		t.Fatal("profile write changed another layout")
	}
	if settings["viewer.profiles.ipad.password"] != "" || settings["viewer.profiles.bad.name.mosaic"] != "" {
		t.Fatal("unexpected writable setting")
	}
}

func TestViewerCameraFitIsIndependentPerProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "secret")
	ctx := context.Background()
	if err := a.UpdateSettings(ctx, map[string]string{
		"viewer.profiles.ipad.fit.cam1":   "cover",
		"viewer.profiles.ipad.fit.cam5":   "contain",
		"viewer.profiles.office.fit.cam1": "cover",
		"camera.display.dev1.fit_mode":    "cover",
	}); err != nil {
		t.Fatal(err)
	}
	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.ProfileFits["ipad"]["cam1"] != "cover" || viewer.ProfileFits["ipad"]["cam5"] != "contain" || viewer.ProfileFits["office"]["cam1"] != "cover" {
		t.Fatalf("unexpected fit modes: %+v", viewer.ProfileFits)
	}
	if err := a.UpdateSettings(ctx, map[string]string{"viewer.profiles.ipad.fit.cam1": "invalid"}); err == nil {
		t.Fatal("invalid fit mode accepted")
	}
	if err := a.UpdateSettings(ctx, map[string]string{"viewer.profiles.ipad.fit.cam1": "contain"}); err != nil {
		t.Fatal(err)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["viewer.profiles.office.fit.cam1"] != "cover" || settings["viewer.profiles.ipad.fit.cam5"] != "contain" || settings["camera.display.dev1.fit_mode"] != "cover" {
		t.Fatal("fit update changed another camera, profile, or global display")
	}
}
