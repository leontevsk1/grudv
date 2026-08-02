package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want             bool
	}{
		{"v1.2.0", "v1.1.9", true},
		{"v1.1.9", "v1.2.0", false},
		{"v2.0.0", "v1.9.9", true},
		{"v1.0.1", "v1.0.1", false},
		{"v1.0.0", "v1.0.1", false},
	}

	for _, c := range cases {
		got, err := isNewerVersion(c.latest, c.current)
		if err != nil {
			t.Fatalf("isNewerVersion(%q, %q) вернул ошибку: %v", c.latest, c.current, err)
		}
		if got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestIsNewerVersionInvalidFormat(t *testing.T) {
	if _, err := isNewerVersion("not-a-version", "v1.0.0"); err == nil {
		t.Fatal("ожидалась ошибка при невалидном формате версии")
	}
}

func TestFetchLatestReleaseParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("неверный заголовок Authorization: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"tag_name": "v1.2.3",
			"assets": [
				{"name": "nup-linux-amd64", "browser_download_url": "https://example.com/nup-linux-amd64"},
				{"name": "nup-linux-arm64", "browser_download_url": "https://example.com/nup-linux-arm64"}
			]
		}`))
	}))
	defer server.Close()

	release, err := fetchLatestReleaseFromURL(server.URL, "test-token")
	if err != nil {
		t.Fatalf("fetchLatestReleaseFromURL вернул ошибку: %v", err)
	}
	if release.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", release.TagName)
	}
	if len(release.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2", len(release.Assets))
	}
}

func TestFetchLatestReleaseNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := fetchLatestReleaseFromURL(server.URL, "test-token"); err == nil {
		t.Fatal("ожидалась ошибка при статусе 404")
	}
}
