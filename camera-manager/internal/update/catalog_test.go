package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCollectNewerReleasesStopsAtCurrent(t *testing.T) {
	payload := `[{"tag_name":"v0.2.0"},{"tag_name":"v0.1.5"},{"tag_name":"v0.1.2"}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	catalog := Catalog{BaseURL: server.URL, Client: server.Client()}
	check := func(current string) (*ReleaseInfo, []ReleaseInfo) {
		t.Helper()
		result, err := catalog.Check(context.Background(), current)
		if err != nil {
			t.Fatal(err)
		}
		return result.Latest, result.Changes
	}
	latest, changes := check("0.1.2")
	if latest == nil || latest.Tag != "v0.2.0" {
		t.Fatalf("unexpected latest %+v", latest)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes since 0.1.2, got %d", len(changes))
	}
	newest, none := check("0.2.0")
	if none != nil {
		t.Fatalf("expected no changes when current is latest, got %+v", none)
	}
	if newest != nil {
		t.Fatalf("expected no latest release when current is newest, got %+v", newest)
	}
	// The installed version carries no "v" prefix, GitHub tags do. They must
	// still compare equal so the UI never advertises the installed release.
	sameTag, sameChanges := check("0.1.5")
	if sameTag == nil || sameTag.Tag != "v0.2.0" {
		t.Fatalf("unexpected latest for 0.1.5: %+v", sameTag)
	}
	if len(sameChanges) != 1 {
		t.Fatalf("expected only v0.2.0 to be newer than 0.1.5, got %+v", sameChanges)
	}
}
