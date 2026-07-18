package main

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type MasterUser struct {
	TgID          int64   `json:"tg_id"`
	Tier          string  `json:"tier"`
	ExpireAt      *string `json:"expire_at"`
	VlessUUID     *string `json:"vless_uuid"`
	NaiveUsername *string `json:"naive_username"`
	NaivePassword *string `json:"naive_password"`
	Hy2Password   *string `json:"hy2_password"`
	TuicUUID      *string `json:"tuic_uuid"`
	TuicPassword  *string `json:"tuic_password"`
}

type MasterNode struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Address        string `json:"address"`
	NodeType       string `json:"node_type"`
	UpstreamNodeID *int   `json:"upstream_node_id"`
	JoinToken      string `json:"join_token"`
}

type MasterConfigResponse struct {
	Node         MasterNode   `json:"node"`
	Users        []MasterUser `json:"users"`
	UpstreamNode *MasterNode  `json:"upstream_node"`
}

// Внутренние типизированные структуры sing-box для маршалинга пользователей
type SingBoxUser struct {
	Name string `json:"name"`
	UUID string `json:"uuid,omitempty"`
	Flow string `json:"flow,omitempty"`
}

type SingBoxHy2User struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}
type SingBoxTuicUser struct {
	Name     string `json:"name"`
	UUID     string `json:"uuid"`
	Password string `json:"password"`
}
type SingBoxNaiveUser struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Имя юзера в sing-box = tg_id: только так v2ray_api отдаёт статистику
// трафика по каждому пользователю отдельно.
func userName(u *MasterUser) string {
	return strconv.FormatInt(u.TgID, 10)
}

func tierNames(users []MasterUser) (premiumNames, freeNames, allNames []string) {
	for i := range users {
		name := userName(&users[i])
		if users[i].Tier == "free" {
			freeNames = append(freeNames, name)
		} else {
			premiumNames = append(premiumNames, name)
		}
		allNames = append(allNames, name)
	}
	return premiumNames, freeNames, allNames
}

// Правила теперь матчатся по спискам имён, а не по имени тарифа,
// поэтому собираются динамически; пустой матчер sing-box не принимает.
func buildTierRules(premiumNames, freeNames []string, premiumOutbound, freeOutbound string, inboundTags []string) []any {
	var rules []any
	if len(premiumNames) > 0 {
		rule := map[string]any{"user": premiumNames, "action": "route", "outbound": premiumOutbound}
		if inboundTags != nil {
			rule["inbound"] = inboundTags
		}
		rules = append(rules, rule)
	}
	if len(freeNames) > 0 {
		rule := map[string]any{"user": freeNames, "action": "route", "outbound": freeOutbound}
		if inboundTags != nil {
			rule["inbound"] = inboundTags
		}
		rules = append(rules, rule)
	}
	return rules
}

func applyRouteRules(configMap map[string]any, tierRules []any) {
	privateBlockRule := map[string]any{"ip_is_private": true, "action": "route", "outbound": "Block"}
	rules := append([]any{privateBlockRule}, tierRules...)
	configMap["route"].(map[string]any)["rules"] = rules
}

// Слушаем 0.0.0.0 внутри контейнера: подман-проброс порта не дотягивается
// до 127.0.0.1 контейнера. Наружу порт публикуется только на 127.0.0.1 хоста.
func applyV2rayAPI(configMap map[string]any, allNames []string) {
	configMap["experimental"] = map[string]any{
		"v2ray_api": map[string]any{
			"listen": "0.0.0.0:8080",
			"stats": map[string]any{
				"enabled": true,
				"users":   allNames,
			},
		},
	}
}

func GenerateConfig(data *MasterConfigResponse) ([]byte, error) {
	switch data.Node.NodeType {
	case "reality":
		return buildRealityConfig(data)
	case "web":
		return buildWebConfig(data)
	case "relay":
		return buildRelayConfig(data)
	default:
		return nil, fmt.Errorf("unknown node type: %s", data.Node.NodeType)
	}
}

// -----------------------------------------------------------------
// 1. DIRECT-REALITY WORKER
// -----------------------------------------------------------------
func buildRealityConfig(data *MasterConfigResponse) ([]byte, error) {
	var vlessUsers []SingBoxUser
	var hy2Users []SingBoxHy2User
	var tuicUsers []SingBoxTuicUser // Исправлен тип

	for i := range data.Users {
		u := &data.Users[i]
		name := userName(u)
		if u.VlessUUID != nil {
			vlessUsers = append(vlessUsers, SingBoxUser{Name: name, UUID: *u.VlessUUID, Flow: "xtls-rprx-vision"})
		}
		if u.Hy2Password != nil {
			hy2Users = append(hy2Users, SingBoxHy2User{Name: name, Password: *u.Hy2Password})
		}
		if u.TuicUUID != nil && u.TuicPassword != nil {
			tuicUsers = append(tuicUsers, SingBoxTuicUser{Name: name, UUID: *u.TuicUUID, Password: *u.TuicPassword})
		}
	}

	// Базовый шаблон на основе нашего reality_worker.json
	baseTemplate := `{
		"log": { "level": "info" },
		"inbounds": [
			{
				"type": "vless",
				"tag": "in-vless-reality",
				"listen": "::",
				"listen_port": 443,
				"tls": {
					"enabled": true,
					"server_name": "telemetry.mozilla.org",
					"reality": {
						"enabled": true,
						"handshake": { "server": "telemetry.mozilla.org", "server_port": 443 },
						"private_key": "<REALITY_PRIVATE_KEY>",
						"short_id": ["<REALITY_SHORT_ID>"]
					}
				}
			},
			{
				"type": "hysteria2",
				"tag": "in-hysteria2",
				"listen": "::",
				"listen_port": 8443,
				"tls": {
					"enabled": true,
					"alpn": ["h3"],
					"certificate_path": "/path/to/fullchain.pem",
					"key_path": "/path/to/privkey.pem"
				}
			},
			{
				"type": "tuic",
				"tag": "in-tuic",
				"listen": "::",
				"listen_port": 443,
				"congestion_control": "bbr",
				"tls": {
					"enabled": true,
					"alpn": ["h3"],
					"certificate_path": "/path/to/fullchain.pem",
					"key_path": "/path/to/privkey.pem"
				}
			}
		],
		"outbounds": [
			{ "type": "direct", "tag": "Direct-Premium" },
			{ "type": "direct", "tag": "Direct-Free", "routing_mark": 100 },
			{ "type": "block", "tag": "Block" }
		],
		"route": {
			"rules": [],
			"final": "Block"
		}
	}`

	var configMap map[string]any
	if err := json.Unmarshal([]byte(baseTemplate), &configMap); err != nil {
		return nil, err
	}

	// Прямолинейно внедряем пользователей в inbounds по индексам шаблона
	inbounds := configMap["inbounds"].([]any)

	vlessInbound := inbounds[0].(map[string]any)
	vlessInbound["users"] = vlessUsers

	hy2Inbound := inbounds[1].(map[string]any)
	hy2Inbound["users"] = hy2Users

	tuicInbound := inbounds[2].(map[string]any)
	tuicInbound["users"] = tuicUsers

	premiumNames, freeNames, allNames := tierNames(data.Users)
	applyRouteRules(configMap, buildTierRules(premiumNames, freeNames, "Direct-Premium", "Direct-Free", nil))
	applyV2rayAPI(configMap, allNames)

	return json.MarshalIndent(configMap, "", "  ")
}

// -----------------------------------------------------------------
// 2. WEBSERVER WORKER (CADDY FRONTEND)
// -----------------------------------------------------------------
func buildWebConfig(data *MasterConfigResponse) ([]byte, error) {
	var vlessUsers []SingBoxUser
	var naiveUsers []SingBoxNaiveUser
	var hy2Users []SingBoxHy2User
	var tuicUsers []SingBoxTuicUser // Исправлен тип

	for i := range data.Users {
		u := &data.Users[i]
		name := userName(u)
		if u.VlessUUID != nil {
			vlessUsers = append(vlessUsers, SingBoxUser{Name: name, UUID: *u.VlessUUID})
		}
		if u.NaiveUsername != nil && u.NaivePassword != nil {
			naiveUsers = append(naiveUsers, SingBoxNaiveUser{Name: name, Username: *u.NaiveUsername, Password: *u.NaivePassword})
		}
		if u.Hy2Password != nil {
			hy2Users = append(hy2Users, SingBoxHy2User{Name: name, Password: *u.Hy2Password})
		}
		if u.TuicUUID != nil && u.TuicPassword != nil {
			tuicUsers = append(tuicUsers, SingBoxTuicUser{Name: name, UUID: *u.TuicUUID, Password: *u.TuicPassword})
		}
	}

	baseTemplate := `{
		"log": { "level": "info" },
		"inbounds": [
			{
				"type": "vless",
				"tag": "in-vless-xhttp",
				"listen": "127.0.0.1",
				"listen_port": 2026,
				"transport": { "type": "httpupgrade", "path": "/your-secret-health-path" }
			},
			{
				"type": "naive",
				"tag": "in-naive",
				"listen": "127.0.0.1",
				"listen_port": 2027
			},
			{
				"type": "hysteria2",
				"tag": "in-hysteria2",
				"listen": "::",
				"listen_port": 8443,
				"tls": {
					"enabled": true,
					"certificate_path": "/path/to/fullchain.pem",
					"key_path": "/path/to/privkey.pem",
					"alpn": ["h3"]
				}
			},
			{
				"type": "tuic",
				"tag": "in-tuic",
				"listen": "::",
				"listen_port": 443,
				"congestion_control": "bbr",
				"tls": {
					"enabled": true,
					"certificate_path": "/path/to/fullchain.pem",
					"key_path": "/path/to/privkey.pem",
					"alpn": ["h3"]
				}
			}
		],
		"outbounds": [
			{ "type": "direct", "tag": "Direct-Premium" },
			{ "type": "direct", "tag": "Direct-Free", "routing_mark": 100 },
			{ "type": "block", "tag": "Block" }
		],
		"route": {
			"rules": [],
			"final": "Block"
		}
	}`

	var configMap map[string]any
	if err := json.Unmarshal([]byte(baseTemplate), &configMap); err != nil {
		return nil, err
	}

	inbounds := configMap["inbounds"].([]any)

	inbounds[0].(map[string]any)["users"] = vlessUsers
	inbounds[1].(map[string]any)["users"] = naiveUsers
	inbounds[2].(map[string]any)["users"] = hy2Users
	inbounds[3].(map[string]any)["users"] = tuicUsers

	premiumNames, freeNames, allNames := tierNames(data.Users)
	applyRouteRules(configMap, buildTierRules(premiumNames, freeNames, "Direct-Premium", "Direct-Free", nil))
	applyV2rayAPI(configMap, allNames)

	return json.MarshalIndent(configMap, "", "  ")
}

// -----------------------------------------------------------------
// 3. RELAY WORKER
// -----------------------------------------------------------------
func buildRelayConfig(data *MasterConfigResponse) ([]byte, error) {
	var vlessInboundUsers []SingBoxUser
	var hy2InboundUsers []SingBoxHy2User
	var tuicInboundUsers []SingBoxTuicUser // Исправлен тип

	for i := range data.Users {
		u := &data.Users[i]
		name := userName(u)
		if u.VlessUUID != nil {
			vlessInboundUsers = append(vlessInboundUsers, SingBoxUser{Name: name, UUID: *u.VlessUUID, Flow: "xtls-rprx-vision"})
		}
		if u.Hy2Password != nil {
			hy2InboundUsers = append(hy2InboundUsers, SingBoxHy2User{Name: name, Password: *u.Hy2Password})
		}
		if u.TuicUUID != nil && u.TuicPassword != nil {
			tuicInboundUsers = append(tuicInboundUsers, SingBoxTuicUser{Name: name, UUID: *u.TuicUUID, Password: *u.TuicPassword})
		}
	}

	baseTemplate := `{
		"log": { "level": "info" },
		"inbounds": [
			{
				"type": "vless",
				"tag": "in-vless-reality",
				"listen": "::",
				"listen_port": 443,
				"tls": {
					"enabled": true,
					"server_name": "telemetry.mozilla.org",
					"reality": {
						"enabled": true,
						"handshake": { "server": "telemetry.mozilla.org", "server_port": 443 },
						"private_key": "<LOCAL_REALITY_PRIVATE_KEY>",
						"short_id": ["<LOCAL_REALITY_SHORT_ID>"]
					}
				}
			},
			{
				"type": "hysteria2",
				"tag": "in-hysteria2",
				"listen": "::",
				"listen_port": 8443,
				"tls": {
					"enabled": true,
					"alpn": ["h3"],
					"certificate_path": "/path/to/fullchain.pem",
					"key_path": "/path/to/privkey.pem"
				}
			},
			{
				"type": "tuic",
				"tag": "in-tuic",
				"listen": "::",
				"listen_port": 443,
				"congestion_control": "bbr",
				"tls": {
					"enabled": true,
					"alpn": ["h3"],
					"certificate_path": "/path/to/fullchain.pem",
					"key_path": "/path/to/privkey.pem"
				}
			}
		],
		"outbounds": [
			{
				"type": "vless",
				"tag": "Upstream-TCP-Premium",
				"server_port": 443,
				"flow": "xtls-rprx-vision",
				"tls": {
					"enabled": true,
					"server_name": "telemetry.mozilla.org",
					"utls": { "enabled": true, "fingerprint": "chrome" },
					"reality": { "enabled": true }
				}
			},
			{
				"type": "vless",
				"tag": "Upstream-TCP-Free",
				"server_port": 443,
				"flow": "xtls-rprx-vision",
				"routing_mark": 100,
				"tls": {
					"enabled": true,
					"server_name": "telemetry.mozilla.org",
					"utls": { "enabled": true, "fingerprint": "chrome" },
					"reality": { "enabled": true }
				}
			},
			{
				"type": "hysteria2",
				"tag": "Upstream-UDP-Premium",
				"server_port": 8443,
				"tls": { "enabled": true, "insecure": false }
			},
			{
				"type": "hysteria2",
				"tag": "Upstream-UDP-Free",
				"server_port": 8443,
				"routing_mark": 100,
				"tls": { "enabled": true, "insecure": false }
			},
			{ "type": "block", "tag": "Block" }
		],
		"route": {
			"rules": [],
			"final": "Block"
		}
	}`

	var configMap map[string]any
	if err := json.Unmarshal([]byte(baseTemplate), &configMap); err != nil {
		return nil, err
	}

	// Наполнение Inbounds
	inbounds := configMap["inbounds"].([]any)
	inbounds[0].(map[string]any)["users"] = vlessInboundUsers
	inbounds[1].(map[string]any)["users"] = hy2InboundUsers
	inbounds[2].(map[string]any)["users"] = tuicInboundUsers

	// Для релея нам жизненно важны параметры вышестоящего целевого воркера (Upstream Node)
	if data.UpstreamNode == nil {
		return nil, fmt.Errorf("relay node requires an upstream node configured")
	}

	// Динамически подставляем адреса и ключи целевого воркера в outbounds релея
	outbounds := configMap["outbounds"].([]any)
	upstream := data.UpstreamNode

	for i := 0; i < 4; i++ {
		outbounds[i].(map[string]any)["server"] = upstream.Address
	}

	// VLESS Upstream
	outbounds[0].(map[string]any)["uuid"] = "<FOREIGN_VLESS_UUID>" // Заменится на uuid мастер-ключа или останется константой
	outbounds[1].(map[string]any)["uuid"] = "<FOREIGN_VLESS_UUID>"

	// Hysteria Upstream
	outbounds[2].(map[string]any)["password"] = "<FOREIGN_HY2_PASSWORD>"
	outbounds[3].(map[string]any)["password"] = "<FOREIGN_HY2_PASSWORD>"

	premiumNames, freeNames, allNames := tierNames(data.Users)
	tcpRules := buildTierRules(premiumNames, freeNames, "Upstream-TCP-Premium", "Upstream-TCP-Free", []string{"in-vless-reality"})
	udpRules := buildTierRules(premiumNames, freeNames, "Upstream-UDP-Premium", "Upstream-UDP-Free", []string{"in-hysteria2", "in-tuic"})
	applyRouteRules(configMap, append(tcpRules, udpRules...))
	applyV2rayAPI(configMap, allNames)

	return json.MarshalIndent(configMap, "", "  ")
}
