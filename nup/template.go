package main

import (
	"encoding/json"
	"fmt"
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
	ID             int         `json:"id"`
	Name           string      `json:"name"`
	Address        string      `json:"address"`
	NodeType       string      `json:"node_type"`
	UpstreamNodeID *int        `json:"upstream_node_id"`
	JoinToken      string      `json:"join_token"`
	RealityPubKey  *string     `json:"reality_pub_key"`
	RealityShortID *string     `json:"reality_short_id"`
	Config         *NodeConfig `json:"config"`
}

// -----------------------------------------------------------------
// ДЕКЛАРАТИВНАЯ КОНФИГУРАЦИЯ УЗЛА (JSONB nodes.config, для reality/web)
// -----------------------------------------------------------------
// Секреты (reality private_key, TLS-сертификаты) сюда не попадают —
// см. buildInbound: они подставляются локально, как и раньше.

type NodeConfig struct {
	Version   int              `json:"version"`
	LogLevel  string           `json:"log_level"`
	Inbounds  []InboundConfig  `json:"inbounds"`
	Outbounds []OutboundConfig `json:"outbounds"`
	Route     RouteConfig      `json:"route"`
}

type InboundConfig struct {
	Tag              string           `json:"tag"`
	Type             string           `json:"type"`
	Listen           string           `json:"listen"`
	ListenPort       int              `json:"listen_port"`
	ProtocolSettings ProtocolSettings `json:"protocol_settings"`
	Transport        *TransportConfig `json:"transport"`
	TLS              *TLSConfig       `json:"tls"`
	UserIDs          []int64          `json:"user_ids"`
	CredentialField  string           `json:"credential_field"`
}

type ProtocolSettings struct {
	Flow               string `json:"flow,omitempty"`
	CongestionControl  string `json:"congestion_control,omitempty"`
}

type TransportConfig struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

type TLSConfig struct {
	Mode       string   `json:"mode"` // "cert" | "reality"
	ServerName string   `json:"server_name,omitempty"`
	Alpn       []string `json:"alpn,omitempty"`
}

type OutboundConfig struct {
	Tag         string `json:"tag"`
	Type        string `json:"type"`
	RoutingMark *int   `json:"routing_mark"`
}

type RouteConfig struct {
	Rules []map[string]any `json:"rules"`
	Final string            `json:"final"`
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

func GenerateConfig(data *MasterConfigResponse, localKeys *RealityLocalKeys) ([]byte, error) {
	switch data.Node.NodeType {
	case "relay":
		return buildRelayConfig(data, localKeys)
	case "reality", "web":
		if data.Node.Config == nil {
			return nil, fmt.Errorf("node has no declarative config yet, configure via PUT /api/v1/nodes/{id}/config")
		}
		return buildDeclarativeConfig(data, localKeys)
	default:
		return nil, fmt.Errorf("unknown node type: %s", data.Node.NodeType)
	}
}

// -----------------------------------------------------------------
// ДЕКЛАРАТИВНЫЙ ИНТЕРПРЕТАТОР (reality/web через nodes.config JSONB)
// -----------------------------------------------------------------

func buildDeclarativeConfig(data *MasterConfigResponse, localKeys *RealityLocalKeys) ([]byte, error) {
	cfg := data.Node.Config
	if cfg.Version != 1 {
		return nil, fmt.Errorf("unsupported node config version: %d", cfg.Version)
	}

	usersByID := make(map[int64]MasterUser, len(data.Users))
	for _, u := range data.Users {
		usersByID[u.TgID] = u
	}

	inbounds := make([]map[string]any, 0, len(cfg.Inbounds))
	for _, ib := range cfg.Inbounds {
		built, err := buildInbound(ib, usersByID, localKeys)
		if err != nil {
			return nil, err
		}
		inbounds = append(inbounds, built)
	}

	outbounds := make([]map[string]any, 0, len(cfg.Outbounds))
	for _, ob := range cfg.Outbounds {
		outbounds = append(outbounds, map[string]any{
			"type":         ob.Type,
			"tag":          ob.Tag,
			"routing_mark": ob.RoutingMark,
		})
	}

	result := map[string]any{
		"log":       map[string]any{"level": cfg.LogLevel},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route": map[string]any{
			"rules": cfg.Route.Rules,
			"final": cfg.Route.Final,
		},
	}

	return json.MarshalIndent(result, "", "  ")
}

func buildInbound(ib InboundConfig, usersByID map[int64]MasterUser, localKeys *RealityLocalKeys) (map[string]any, error) {
	inbound := map[string]any{
		"type":        ib.Type,
		"tag":         ib.Tag,
		"listen":      ib.Listen,
		"listen_port": ib.ListenPort,
	}

	if ib.ProtocolSettings.Flow != "" {
		inbound["flow"] = ib.ProtocolSettings.Flow
	}
	if ib.ProtocolSettings.CongestionControl != "" {
		inbound["congestion_control"] = ib.ProtocolSettings.CongestionControl
	}

	if ib.Transport != nil {
		inbound["transport"] = map[string]any{
			"type": ib.Transport.Type,
			"path": ib.Transport.Path,
		}
	}

	if ib.TLS != nil {
		tls := map[string]any{"enabled": true}
		if len(ib.TLS.Alpn) > 0 {
			tls["alpn"] = ib.TLS.Alpn
		}

		switch ib.TLS.Mode {
		case "cert":
			tls["certificate_path"] = "/etc/sing-box/fullchain.pem"
			tls["key_path"] = "/etc/sing-box/privkey.pem"
		case "reality":
			tls["server_name"] = ib.TLS.ServerName
			tls["reality"] = map[string]any{
				"enabled": true,
				"handshake": map[string]any{
					"server":      ib.TLS.ServerName,
					"server_port": 443,
				},
			}
		default:
			return nil, fmt.Errorf("inbound %q: unknown tls mode %q", ib.Tag, ib.TLS.Mode)
		}

		inbound["tls"] = tls

		if ib.TLS.Mode == "reality" {
			if err := setRealityKeys(inbound, localKeys); err != nil {
				return nil, err
			}
		}
	}

	users, err := buildInboundUsers(ib, usersByID)
	if err != nil {
		return nil, err
	}
	inbound["users"] = users

	return inbound, nil
}

func buildInboundUsers(ib InboundConfig, usersByID map[int64]MasterUser) (any, error) {
	switch ib.CredentialField {
	case "vless":
		var out []SingBoxUser
		for _, id := range ib.UserIDs {
			if u, ok := usersByID[id]; ok && u.VlessUUID != nil {
				out = append(out, SingBoxUser{Name: u.Tier, UUID: *u.VlessUUID, Flow: ib.ProtocolSettings.Flow})
			}
		}
		return out, nil
	case "hy2":
		var out []SingBoxHy2User
		for _, id := range ib.UserIDs {
			if u, ok := usersByID[id]; ok && u.Hy2Password != nil {
				out = append(out, SingBoxHy2User{Name: u.Tier, Password: *u.Hy2Password})
			}
		}
		return out, nil
	case "tuic":
		var out []SingBoxTuicUser
		for _, id := range ib.UserIDs {
			if u, ok := usersByID[id]; ok && u.TuicUUID != nil && u.TuicPassword != nil {
				out = append(out, SingBoxTuicUser{Name: u.Tier, UUID: *u.TuicUUID, Password: *u.TuicPassword})
			}
		}
		return out, nil
	case "naive":
		var out []SingBoxNaiveUser
		for _, id := range ib.UserIDs {
			if u, ok := usersByID[id]; ok && u.NaiveUsername != nil && u.NaivePassword != nil {
				out = append(out, SingBoxNaiveUser{Name: u.Tier, Username: *u.NaiveUsername, Password: *u.NaivePassword})
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("inbound %q: unknown credential_field %q", ib.Tag, ib.CredentialField)
	}
}

// -----------------------------------------------------------------
// RELAY WORKER (пока не переведён на декларативный config, см. CLAUDE.md)
// -----------------------------------------------------------------
func buildRelayConfig(data *MasterConfigResponse, localKeys *RealityLocalKeys) ([]byte, error) {
	var vlessInboundUsers []SingBoxUser
	var hy2InboundUsers []SingBoxHy2User
	var tuicInboundUsers []SingBoxTuicUser // Исправлен тип

	for _, u := range data.Users {
		if u.VlessUUID != nil {
			vlessInboundUsers = append(vlessInboundUsers, SingBoxUser{Name: u.Tier, UUID: *u.VlessUUID, Flow: "xtls-rprx-vision"})
		}
		if u.Hy2Password != nil {
			hy2InboundUsers = append(hy2InboundUsers, SingBoxHy2User{Name: u.Tier, Password: *u.Hy2Password})
		}
		if u.TuicUUID != nil && u.TuicPassword != nil {
			// Используем правильную структуру SingBoxTuicUser
			tuicInboundUsers = append(tuicInboundUsers, SingBoxTuicUser{Name: u.Tier, UUID: *u.TuicUUID, Password: *u.TuicPassword})
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
					"certificate_path": "/etc/sing-box/fullchain.pem",
					"key_path": "/etc/sing-box/privkey.pem"
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
					"certificate_path": "/etc/sing-box/fullchain.pem",
					"key_path": "/etc/sing-box/privkey.pem"
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
			"rules": [
				{ "ip_is_private": true, "action": "route", "outbound": "Block" },
				{ "inbound": ["in-vless-reality"], "auth_user": ["premium"], "action": "route", "outbound": "Upstream-TCP-Premium" },
				{ "inbound": ["in-vless-reality"], "auth_user": ["free"], "action": "route", "outbound": "Upstream-TCP-Free" },
				{ "inbound": ["in-hysteria2", "in-tuic"], "auth_user": ["premium"], "action": "route", "outbound": "Upstream-UDP-Premium" },
				{ "inbound": ["in-hysteria2", "in-tuic"], "auth_user": ["free"], "action": "route", "outbound": "Upstream-UDP-Free" }
			],
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
	if err := setRealityKeys(inbounds[0].(map[string]any), localKeys); err != nil {
		return nil, err
	}
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

	if upstream.RealityPubKey == nil || *upstream.RealityPubKey == "" {
		return nil, fmt.Errorf("upstream node %d has no reality public key yet", upstream.ID)
	}
	for i := 0; i < 2; i++ {
		reality := outbounds[i].(map[string]any)["tls"].(map[string]any)["reality"].(map[string]any)
		reality["public_key"] = *upstream.RealityPubKey
	}

	// VLESS Upstream
	outbounds[0].(map[string]any)["uuid"] = "<FOREIGN_VLESS_UUID>" // Заменится на uuid мастер-ключа или останется константой
	outbounds[1].(map[string]any)["uuid"] = "<FOREIGN_VLESS_UUID>"

	// Hysteria Upstream
	outbounds[2].(map[string]any)["password"] = "<FOREIGN_HY2_PASSWORD>"
	outbounds[3].(map[string]any)["password"] = "<FOREIGN_HY2_PASSWORD>"

	return json.MarshalIndent(configMap, "", "  ")
}
