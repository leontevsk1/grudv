package main

import (
	"os"
)

type Config struct {
	MasterURL        string // URL нашего vpn-core (например, http://127.0.0.1:8443)
	NodeJoinToken    string // Статичный Bearer токен для авторизации на Мастере
	NodeType         string // 'reality', 'web', 'relay'
	Domain           string // Публичный домен или IP этого воркера
	ConfigPath       string // Путь к config.json для sing-box
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

	return &Config{
		MasterURL:     masterURL,
		NodeJoinToken: os.Getenv("NODE_JOIN_TOKEN"),
		NodeType:      nodeType,
		Domain:        os.Getenv("DOMAIN"),
		ConfigPath:    configPath,
	}
}
