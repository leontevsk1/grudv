package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type TrafficReport struct {
	TgID  int64 `json:"tg_id"`
	Bytes int64 `json:"bytes"`
}

// Сообщения StatsService состоят из двух-трёх полей, поэтому protobuf
// кодируется руками — это избавляет от protoc-кодогенерации и тяжёлых зависимостей.
type passthroughCodec struct{}

func (passthroughCodec) Marshal(v any) ([]byte, error) {
	return v.([]byte), nil
}

func (passthroughCodec) Unmarshal(data []byte, v any) error {
	*(v.(*[]byte)) = data
	return nil
}

// Имя "proto" оставляет стандартный content-type application/grpc+proto,
// иначе сервер отклонит вызов.
func (passthroughCodec) Name() string {
	return "proto"
}

func encodeQueryStatsRequest(pattern string, reset bool) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x0a)
	buf.Write(binary.AppendUvarint(nil, uint64(len(pattern))))
	buf.WriteString(pattern)
	if reset {
		buf.WriteByte(0x10)
		buf.WriteByte(0x01)
	}
	return buf.Bytes()
}

func decodeStat(data []byte) (string, int64, error) {
	var name string
	var value int64

	pos := 0
	for pos < len(data) {
		tag := data[pos]
		pos++
		switch tag {
		case 0x0a:
			length, n := binary.Uvarint(data[pos:])
			if n <= 0 || pos+n+int(length) > len(data) {
				return "", 0, fmt.Errorf("malformed stat name")
			}
			pos += n
			name = string(data[pos : pos+int(length)])
			pos += int(length)
		case 0x10:
			v, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				return "", 0, fmt.Errorf("malformed stat value")
			}
			pos += n
			value = int64(v)
		default:
			return "", 0, fmt.Errorf("unexpected stat field tag 0x%x", tag)
		}
	}

	return name, value, nil
}

func decodeQueryStatsResponse(data []byte) (map[string]int64, error) {
	stats := make(map[string]int64)

	pos := 0
	for pos < len(data) {
		if data[pos] != 0x0a {
			return nil, fmt.Errorf("unexpected response field tag 0x%x", data[pos])
		}
		pos++
		length, n := binary.Uvarint(data[pos:])
		if n <= 0 || pos+n+int(length) > len(data) {
			return nil, fmt.Errorf("malformed response")
		}
		pos += n
		name, value, err := decodeStat(data[pos : pos+int(length)])
		if err != nil {
			return nil, err
		}
		pos += int(length)
		stats[name] = value
	}

	return stats, nil
}

// Суммирует uplink+downlink по каждому юзеру.
// Имена счётчиков: user>>>{tg_id}>>>traffic>>>uplink|downlink.
func aggregateUserTraffic(stats map[string]int64) []TrafficReport {
	totals := make(map[int64]int64)
	for name, value := range stats {
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || value <= 0 {
			continue
		}
		tgID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		totals[tgID] += value
	}

	var reports []TrafficReport
	for tgID, total := range totals {
		reports = append(reports, TrafficReport{TgID: tgID, Bytes: total})
	}
	return reports
}

func queryUserStats(addr string) (map[string]int64, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// reset=true: sing-box обнуляет счётчики, на мастер уходят только дельты
	request := encodeQueryStatsRequest("user>>>", true)
	var response []byte
	err = conn.Invoke(
		ctx,
		"/v2ray.core.app.stats.command.StatsService/QueryStats",
		request,
		&response,
		grpc.ForceCodec(passthroughCodec{}),
	)
	if err != nil {
		return nil, err
	}

	return decodeQueryStatsResponse(response)
}

func CollectAndReportTraffic(cfg *Config) {
	stats, err := queryUserStats(cfg.StatsAddr)
	if err != nil {
		log.Printf("Ошибка запроса статистики sing-box: %v", err)
		return
	}

	reports := aggregateUserTraffic(stats)
	if len(reports) == 0 {
		return
	}

	data, err := json.Marshal(reports)
	if err != nil {
		log.Printf("Ошибка сериализации отчёта о трафике: %v", err)
		return
	}

	req, err := http.NewRequest("POST", cfg.MasterURL+"/api/v1/nup/traffic", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Ошибка формирования запроса трафика: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.NodeJoinToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Ошибка отправки трафика на Мастер: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Мастер отклонил отчёт о трафике: %d", resp.StatusCode)
	}
}
