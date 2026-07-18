package main

import (
	"log"
	"os"
	"os/exec"
)

type Config struct {
	MasterURL        string // URL нашего vpn-core (например, http://127.0.0.1:8443)
	NodeJoinToken    string // Статичный Bearer токен для авторизации на Мастере
	NodeType         string // 'reality', 'web', 'relay'
	Domain           string // Публичный домен или IP этого воркера
	ConfigPath       string // Путь к config.json для sing-box
	ContainerEngine  string // 'podman' или 'docker' — определяется автоматически
	ContainerName    string // Имя контейнера sing-box для restart
}

func LoadConfig() *Config {
	masterURL := os.Getenv("MASTER_URL")
	if masterURL == "" {
		masterURL = "http://127.0.0.1:8443"
	}

	nodeType := os.Getenv("NODE_TYPE")
	if nodeType == "" {
		nodeType = "reality" // Дефолт по ТЗ
	}

	configPath := os.Getenv("NUP_CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/sing-box/config.json" // Дефолт для боевой системы
	}

	containerName := os.Getenv("SINGBOX_CONTAINER_NAME")
	if containerName == "" {
		containerName = "sing-box-worker"
	}

	return &Config{
		MasterURL:       masterURL,
		NodeJoinToken:   os.Getenv("NODE_JOIN_TOKEN"),
		NodeType:        nodeType,
		Domain:          os.Getenv("DOMAIN"),
		ConfigPath:      configPath,
		ContainerEngine: detectContainerEngine(),
		ContainerName:   containerName,
	}
}

// detectContainerEngine сканирует систему на наличие бинарников движков.
// podman в приоритете — на нём ушла в архитектуру исходная система (rootless, systemd-friendly).
func detectContainerEngine() string {
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	log.Fatal("Критическая ошибка: не найден ни podman, ни docker в PATH.")
	return ""
}
