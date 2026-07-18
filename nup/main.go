package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func main() {
	log.Println("Запуск Go-демона nup...")
	cfg := LoadConfig()

	if cfg.NodeJoinToken == "" {
		log.Fatal("Критическая ошибка: NODE_JOIN_TOKEN не задан в окружении.")
	}

	// Запускаем бесконечный цикл с тикером в 1 минуту по ТЗ
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Первый запуск при старте демона
	fetchAndApplyConfig(cfg)

	for range ticker.C {
		// Сначала снимаем статистику: рестарт контейнера при обновлении
		// конфига обнуляет счётчики sing-box.
		CollectAndReportTraffic(cfg)
		fetchAndApplyConfig(cfg)
	}
}

func fetchAndApplyConfig(cfg *Config) {
	url := fmt.Sprintf("%s/api/v1/nup/config", cfg.MasterURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Ошибка формирования запроса: %v", err)
		return
	}

	// Авторизация по статичному токену из ТЗ
	req.Header.Set("Authorization", "Bearer "+cfg.NodeJoinToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Ошибка выполнения запроса к Мастеру: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		log.Println("Ошибка: Мастер отклонил JOIN_TOKEN (413/401 Unauthorized).")
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Мастер вернул ошибку сервера: %d", resp.StatusCode)
		return
	}

	var masterData MasterConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&masterData); err != nil {
		log.Printf("Ошибка декодирования ответа Мастера: %v", err)
		return
	}

	// Генерируем итоговый конфиг для sing-box
	newConfigBytes, err := GenerateConfig(&masterData)
	if err != nil {
		log.Printf("Ошибка генерации конфигурации: %v", err)
		return
	}

	configPath := cfg.ConfigPath

	// Проверяем, изменился ли конфиг, чтобы зря не дергать Podman
	oldConfigBytes, err := os.ReadFile(configPath)
	if err == nil && bytes.Equal(oldConfigBytes, newConfigBytes) {
		// Конфиг идентичен, ничего не делаем
		return
	}

	// Записываем новый конфиг
	if err := os.WriteFile(configPath, newConfigBytes, 0644); err != nil {
		log.Printf("Критическая ошибка записи config.json: %v", err)
		return
	}
	log.Println("Конфигурация sing-box обновлена. Перезапуск контейнера...")

	// Топорный императивный перезапуск контейнера через bash по ТЗ
	cmd := exec.Command("podman", "restart", "sing-box-backup")
	if err := cmd.Run(); err != nil {
		log.Printf("Ошибка перезапуска контейнера sing-box: %v", err)
	} else {
		log.Println("Контейнер sing-box-backup успешно перезапущен.")
	}

	ApplyTrafficShaping(masterData.Users)
}
