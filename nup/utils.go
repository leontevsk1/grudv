package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// ApplyTrafficShaping настраивает tc лимиты на основе присутствия free/expired юзеров
func ApplyTrafficShaping(users []MasterUser) {
	// 1. Автоматически определяем дефолтный сетевой интерфейс (обычно eth0 или enp1s0)
	iface, err := getDefaultInterface()
	if err != nil {
		log.Printf("Ошибка определения сетевого интерфейса для tc: %v", err)
		return
	}

	// Проверяем, есть ли вообще пользователи, которых нужно душить
	hasFreeOrExpired := false
	now := time.Now()

	for _, u := range users {
		if u.Tier == "free" {
			hasFreeOrExpired = true
			break
		}
		if u.ExpireAt != nil {
			// Наш формат времени в базе — NaiveDateTime (YYYY-MM-DDTHH:MM:SS)
			t, err := time.Parse("2006-01-02T15:04:05", *u.ExpireAt)
			if err == nil && t.Before(now) {
				hasFreeOrExpired = true
				break
			}
		}
	}

	// Полностью очищаем старые правила tc на интерфейсе (игнорируем ошибку, если правил не было)
	_ = exec.Command("tc", "qdisc", "del", "dev", iface, "root").Run()

	if !hasFreeOrExpired {
		// Если душить некого, оставляем интерфейс чистым
		return
	}

	log.Printf("Обнаружены free/expired пользователи. Активация tc троттлинга на %s...", iface)

	// 2. Создаем корневую дисциплину очередей HTB
	if err := exec.Command("tc", "qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "10").Run(); err != nil {
		log.Printf("tc root qdisc error: %v", err)
		return
	}

	// 3. Класс 1:10 — для обычного (Premium) трафика (без ограничений, отдаем условный 1 Гбит)
	_ = exec.Command("tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:10", "htb", "rate", "1gbit").Run()

	// 4. Класс 1:20 — для Free/Expired трафика (жесткий лимит 512 Кбит/с по ТЗ)
	if err := exec.Command("tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:20", "htb", "rate", "512kbit", "ceil", "512kbit").Run(); err != nil {
		log.Printf("tc class free error: %v", err)
		return
	}

	// 5. Связываем маркировку пакетов sing-box (routing_mark 100) с классом ограничения 1:20
	if err := exec.Command("tc", "filter", "add", "dev", iface, "protocol", "ip", "parent", "1:", "prio", "1", "handle", "100", "fw", "flowid", "1:20").Run(); err != nil {
		log.Printf("tc filter error: %v", err)
		return
	}

	log.Println("tc правила успешно применены.")
}

// Вспомогательная функция для императивного получения имени интерфейса из /proc/net/route
func getDefaultInterface() (string, error) {
	output, err := exec.Command("ip", "route", "show", "to", "default").Output()
	if err != nil {
		return "", err
	}

	// Строка выглядит так: "default via 192.168.1.1 dev eth0 proto dhcp src..."
	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}

	return "", fmt.Errorf("default interface not found in routing table")
}
