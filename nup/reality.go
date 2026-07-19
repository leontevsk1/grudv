package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type RealityLocalKeys struct {
	PrivateKey string
	PublicKey  string
	ShortID    string
}

// EnsureRealityLocalKeys возвращает локальную пару ключей узла, генерируя и
// сохраняя её на диск при первом запуске. Приватный ключ никогда не покидает
// воркер — на мастер уходит только публичная часть, см. pushRealityPublicKey.
func EnsureRealityLocalKeys(keyPath string) (*RealityLocalKeys, error) {
	if keys, err := loadRealityLocalKeys(keyPath); err == nil {
		return keys, nil
	}

	keys, err := generateRealityLocalKeys()
	if err != nil {
		return nil, fmt.Errorf("генерация reality keypair: %w", err)
	}

	if err := saveRealityLocalKeys(keyPath, keys); err != nil {
		return nil, fmt.Errorf("сохранение reality keypair: %w", err)
	}

	return keys, nil
}

func loadRealityLocalKeys(keyPath string) (*RealityLocalKeys, error) {
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	var keys RealityLocalKeys
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, err
	}

	return &keys, nil
}

func saveRealityLocalKeys(keyPath string, keys *RealityLocalKeys) error {
	raw, err := json.Marshal(keys)
	if err != nil {
		return err
	}

	return os.WriteFile(keyPath, raw, 0600)
}

func generateRealityLocalKeys() (*RealityLocalKeys, error) {
	keypairOut, err := exec.Command("sing-box", "generate", "reality-keypair").Output()
	if err != nil {
		return nil, fmt.Errorf("sing-box generate reality-keypair: %w", err)
	}

	privateKey, publicKey, err := parseRealityKeypair(string(keypairOut))
	if err != nil {
		return nil, err
	}

	shortIDOut, err := exec.Command("sing-box", "generate", "rand", "8", "--hex").Output()
	if err != nil {
		return nil, fmt.Errorf("sing-box generate rand: %w", err)
	}

	return &RealityLocalKeys{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		ShortID:    strings.TrimSpace(string(shortIDOut)),
	}, nil
}

func parseRealityKeypair(output string) (privateKey, publicKey string, err error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "PrivateKey:"):
			privateKey = strings.TrimSpace(strings.TrimPrefix(line, "PrivateKey:"))
		case strings.HasPrefix(line, "PublicKey:"):
			publicKey = strings.TrimSpace(strings.TrimPrefix(line, "PublicKey:"))
		}
	}

	if privateKey == "" || publicKey == "" {
		return "", "", fmt.Errorf("не удалось распарсить вывод sing-box generate reality-keypair: %q", output)
	}

	return privateKey, publicKey, nil
}

// PushRealityPublicKey однократно отправляет публичную часть ключа на мастер,
// чтобы остальные узлы (например, relay) могли забрать её через /nup/config.
func PushRealityPublicKey(masterURL, joinToken string, keys *RealityLocalKeys) error {
	body, err := json.Marshal(map[string]string{
		"public_key": keys.PublicKey,
		"short_id":   keys.ShortID,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/nup/node-keys", masterURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+joinToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("мастер отклонил reality-ключи узла: %d", resp.StatusCode)
	}

	return nil
}

// setRealityKeys подставляет локальный приватный ключ и short_id узла в
// reality-секцию inbound вместо заглушек шаблона.
func setRealityKeys(inbound map[string]any, keys *RealityLocalKeys) error {
	if keys == nil {
		return fmt.Errorf("reality inbound требует локальные ключи узла, но они не были загружены")
	}

	reality := inbound["tls"].(map[string]any)["reality"].(map[string]any)
	reality["private_key"] = keys.PrivateKey
	reality["short_id"] = []string{keys.ShortID}

	return nil
}
