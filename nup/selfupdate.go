package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckAndSelfUpdate сравнивает текущую версию с последним GitHub Release
// и при отставании скачивает новый бинарник поверх себя и рестартует сервис.
func CheckAndSelfUpdate(cfg *Config, currentVersion string) error {
	if currentVersion == "dev" {
		return nil
	}

	release, err := fetchLatestRelease(cfg)
	if err != nil {
		return fmt.Errorf("запрос последнего релиза: %w", err)
	}

	newer, err := isNewerVersion(release.TagName, currentVersion)
	if err != nil {
		return fmt.Errorf("сравнение версий (%s -> %s): %w", currentVersion, release.TagName, err)
	}
	if !newer {
		return nil
	}

	log.Printf("Найдена новая версия nup: %s -> %s. Обновляюсь...", currentVersion, release.TagName)

	assetName := fmt.Sprintf("nup-linux-%s", goArchToAssetArch(runtime.GOARCH))
	downloadURL := ""
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("в релизе %s не найден ассет %s", release.TagName, assetName)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("определение пути текущего бинарника: %w", err)
	}

	newPath := execPath + ".new"
	if err := downloadFile(cfg.GhToken, downloadURL, newPath); err != nil {
		return fmt.Errorf("скачивание нового бинарника: %w", err)
	}

	if err := os.Chmod(newPath, 0755); err != nil {
		return fmt.Errorf("chmod нового бинарника: %w", err)
	}

	if err := os.Rename(newPath, execPath); err != nil {
		return fmt.Errorf("замена бинарника: %w", err)
	}

	log.Printf("Бинарник nup обновлён до %s. Перезапуск сервиса...", release.TagName)
	if err := exec.Command("systemctl", "restart", "nup.service").Run(); err != nil {
		return fmt.Errorf("systemctl restart nup.service: %w", err)
	}

	return nil
}

func fetchLatestRelease(cfg *Config) (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GhRepo)
	return fetchLatestReleaseFromURL(url, cfg.GhToken)
}

func fetchLatestReleaseFromURL(url, ghToken string) (*githubRelease, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ghToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API вернул статус %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadFile(ghToken, url, destPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+ghToken)
	req.Header.Set("Accept", "application/octet-stream")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("скачивание вернуло статус %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func goArchToAssetArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

// isNewerVersion сравнивает теги вида "vMAJOR.MINOR.PATCH".
func isNewerVersion(latest, current string) (bool, error) {
	lMajor, lMinor, lPatch, err := parseVersion(latest)
	if err != nil {
		return false, err
	}
	cMajor, cMinor, cPatch, err := parseVersion(current)
	if err != nil {
		return false, err
	}

	if lMajor != cMajor {
		return lMajor > cMajor, nil
	}
	if lMinor != cMinor {
		return lMinor > cMinor, nil
	}
	return lPatch > cPatch, nil
}

func parseVersion(v string) (int, int, int, error) {
	var major, minor, patch int
	_, err := fmt.Sscanf(v, "v%d.%d.%d", &major, &minor, &patch)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("неверный формат версии %q: %w", v, err)
	}
	return major, minor, patch, nil
}
