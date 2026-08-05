package main

import (
	"encoding/json"
	"testing"
)

func strPtr(s string) *string { return &s }

func testUsers() []MasterUser {
	return []MasterUser{
		{
			TgID:          1,
			Tier:          "premium",
			VlessUUID:     strPtr("uuid-premium"),
			Hy2Password:   strPtr("hy2-premium"),
			TuicUUID:      strPtr("tuic-uuid-premium"),
			TuicPassword:  strPtr("tuic-pass-premium"),
			NaiveUsername: strPtr("naive-user"),
			NaivePassword: strPtr("naive-pass"),
		},
		{
			TgID:      2,
			Tier:      "free",
			VlessUUID: strPtr("uuid-free"),
		},
	}
}

func testUserIDs() []int64 { return []int64{1, 2} }

func decodeInbounds(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("GenerateConfig produced invalid JSON: %v", err)
	}
	inboundsAny, ok := config["inbounds"].([]any)
	if !ok {
		t.Fatalf("config has no inbounds array")
	}
	inbounds := make([]map[string]any, len(inboundsAny))
	for i, in := range inboundsAny {
		inbounds[i] = in.(map[string]any)
	}
	return inbounds
}

func testLocalKeys() *RealityLocalKeys {
	return &RealityLocalKeys{
		PrivateKey: "local-private-key",
		PublicKey:  "local-public-key",
		ShortID:    "deadbeef",
	}
}

func realityNodeConfig() *NodeConfig {
	return &NodeConfig{
		Version:  1,
		LogLevel: "info",
		Inbounds: []InboundConfig{
			{
				Tag: "in-vless-reality", Type: "vless", Listen: "::", ListenPort: 443,
				ProtocolSettings: ProtocolSettings{Flow: "xtls-rprx-vision"},
				TLS:              &TLSConfig{Mode: "reality", ServerName: "telemetry.mozilla.org"},
				UserIDs:          testUserIDs(),
				CredentialField:  "vless",
			},
			{
				Tag: "in-hysteria2", Type: "hysteria2", Listen: "::", ListenPort: 8443,
				TLS:             &TLSConfig{Mode: "cert", Alpn: []string{"h3"}},
				UserIDs:         testUserIDs(),
				CredentialField: "hy2",
			},
			{
				Tag: "in-tuic", Type: "tuic", Listen: "::", ListenPort: 443,
				ProtocolSettings: ProtocolSettings{CongestionControl: "bbr"},
				TLS:              &TLSConfig{Mode: "cert", Alpn: []string{"h3"}},
				UserIDs:          testUserIDs(),
				CredentialField:  "tuic",
			},
		},
		Outbounds: []OutboundConfig{
			{Tag: "Direct-Premium", Type: "direct"},
			{Tag: "Direct-Free", Type: "direct", RoutingMark: intPtr(100)},
			{Tag: "Block", Type: "block"},
		},
		Route: RouteConfig{
			Rules: []map[string]any{
				{"ip_is_private": true, "action": "route", "outbound": "Block"},
				{"auth_user": []string{"premium"}, "action": "route", "outbound": "Direct-Premium"},
				{"auth_user": []string{"free"}, "action": "route", "outbound": "Direct-Free"},
			},
			Final: "Block",
		},
	}
}

func intPtr(i int) *int { return &i }

func TestGenerateConfigReality(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "reality", Config: realityNodeConfig()},
		Users: testUsers(),
	}

	raw, err := GenerateConfig(data, testLocalKeys())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inbounds := decodeInbounds(t, raw)
	if len(inbounds) != 3 {
		t.Fatalf("got %d inbounds, want 3 (vless, hysteria2, tuic)", len(inbounds))
	}

	vlessUsers := inbounds[0]["users"].([]any)
	if len(vlessUsers) != 2 {
		t.Errorf("vless users = %d, want 2", len(vlessUsers))
	}

	hy2Users := inbounds[1]["users"].([]any)
	if len(hy2Users) != 1 {
		t.Errorf("hy2 users = %d, want 1 (only premium has hy2_password)", len(hy2Users))
	}

	tuicUsers := inbounds[2]["users"].([]any)
	if len(tuicUsers) != 1 {
		t.Errorf("tuic users = %d, want 1 (only premium has tuic creds)", len(tuicUsers))
	}

	reality := inbounds[0]["tls"].(map[string]any)["reality"].(map[string]any)
	if reality["private_key"] != "local-private-key" {
		t.Errorf("reality.private_key = %v, want local-private-key", reality["private_key"])
	}

	assertRoutesByAuthUser(t, raw)
}

// "user" в route rules матчит имя процесса ОС клиента (metadata.ProcessInfo.UserName),
// не аутентифицированного VLESS/HY2/TUIC-пользователя — с ним free/premium маршрутизация
// никогда не срабатывает на сервере. Нужно именно "auth_user" (metadata.User).
func assertRoutesByAuthUser(t *testing.T, raw []byte) {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("GenerateConfig produced invalid JSON: %v", err)
	}

	rules := config["route"].(map[string]any)["rules"].([]any)
	foundAuthUser := false
	for _, r := range rules {
		rule := r.(map[string]any)
		if _, bad := rule["user"]; bad {
			t.Fatalf("route rule uses \"user\" (OS process name) instead of \"auth_user\": %v", rule)
		}
		if _, ok := rule["auth_user"]; ok {
			foundAuthUser = true
		}
	}
	if !foundAuthUser {
		t.Fatal("expected at least one route rule matching by auth_user")
	}
}

func TestGenerateConfigEmptyUserIDsMeansAllUsers(t *testing.T) {
	config := realityNodeConfig()
	config.Inbounds[0].UserIDs = nil

	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "reality", Config: config},
		Users: testUsers(),
	}

	raw, err := GenerateConfig(data, testLocalKeys())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inbounds := decodeInbounds(t, raw)
	vlessUsers := inbounds[0]["users"].([]any)
	if len(vlessUsers) != 2 {
		t.Errorf("vless users = %d, want 2 (nil user_ids should include every user with vless creds)", len(vlessUsers))
	}
}

func webNodeConfig() *NodeConfig {
	return &NodeConfig{
		Version:  1,
		LogLevel: "info",
		Inbounds: []InboundConfig{
			{
				Tag: "in-vless-xhttp", Type: "vless", Listen: "127.0.0.1", ListenPort: 2026,
				Transport:       &TransportConfig{Type: "httpupgrade", Path: "/your-secret-health-path"},
				UserIDs:         testUserIDs(),
				CredentialField: "vless",
			},
			{
				Tag: "in-naive", Type: "naive", Listen: "127.0.0.1", ListenPort: 2027,
				UserIDs:         testUserIDs(),
				CredentialField: "naive",
			},
			{
				Tag: "in-hysteria2", Type: "hysteria2", Listen: "::", ListenPort: 8443,
				TLS:             &TLSConfig{Mode: "cert", Alpn: []string{"h3"}},
				UserIDs:         testUserIDs(),
				CredentialField: "hy2",
			},
			{
				Tag: "in-tuic", Type: "tuic", Listen: "::", ListenPort: 443,
				ProtocolSettings: ProtocolSettings{CongestionControl: "bbr"},
				TLS:              &TLSConfig{Mode: "cert", Alpn: []string{"h3"}},
				UserIDs:          testUserIDs(),
				CredentialField:  "tuic",
			},
		},
		Outbounds: []OutboundConfig{
			{Tag: "Direct-Premium", Type: "direct"},
			{Tag: "Direct-Free", Type: "direct", RoutingMark: intPtr(100)},
			{Tag: "Block", Type: "block"},
		},
		Route: RouteConfig{
			Rules: []map[string]any{
				{"ip_is_private": true, "action": "route", "outbound": "Block"},
				{"auth_user": []string{"premium"}, "action": "route", "outbound": "Direct-Premium"},
				{"auth_user": []string{"free"}, "action": "route", "outbound": "Direct-Free"},
			},
			Final: "Block",
		},
	}
}

func TestGenerateConfigWeb(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "web", Config: webNodeConfig()},
		Users: testUsers(),
	}

	raw, err := GenerateConfig(data, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inbounds := decodeInbounds(t, raw)
	if len(inbounds) != 4 {
		t.Fatalf("got %d inbounds, want 4 (vless, naive, hysteria2, tuic)", len(inbounds))
	}

	naiveUsers := inbounds[1]["users"].([]any)
	if len(naiveUsers) != 1 {
		t.Errorf("naive users = %d, want 1 (only premium has naive creds)", len(naiveUsers))
	}

	assertRoutesByAuthUser(t, raw)
}

func TestGenerateConfigMissingDeclarativeConfig(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "reality", Config: nil},
		Users: testUsers(),
	}

	_, err := GenerateConfig(data, testLocalKeys())
	if err == nil {
		t.Fatal("expected error when reality node has no declarative config, got nil")
	}
}

func TestGenerateConfigUnsupportedVersion(t *testing.T) {
	cfg := realityNodeConfig()
	cfg.Version = 2
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "reality", Config: cfg},
		Users: testUsers(),
	}

	_, err := GenerateConfig(data, testLocalKeys())
	if err == nil {
		t.Fatal("expected error for unsupported config version, got nil")
	}
}

func TestGenerateConfigRelayRequiresUpstream(t *testing.T) {
	data := &MasterConfigResponse{
		Node:         MasterNode{NodeType: "relay"},
		Users:        testUsers(),
		UpstreamNode: nil,
	}

	_, err := GenerateConfig(data, testLocalKeys())
	if err == nil {
		t.Fatal("expected error when relay node has no upstream, got nil")
	}
}

func TestGenerateConfigRelayWithUpstream(t *testing.T) {
	upstreamPubKey := "upstream-public-key"
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "relay"},
		Users: testUsers(),
		UpstreamNode: &MasterNode{
			Address:       "upstream.example.com",
			RealityPubKey: &upstreamPubKey,
		},
	}

	raw, err := GenerateConfig(data, testLocalKeys())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("GenerateConfig produced invalid JSON: %v", err)
	}

	outbounds := config["outbounds"].([]any)
	for i := 0; i < 2; i++ {
		reality := outbounds[i].(map[string]any)["tls"].(map[string]any)["reality"].(map[string]any)
		if reality["public_key"] != upstreamPubKey {
			t.Errorf("outbound[%d].tls.reality.public_key = %v, want %v", i, reality["public_key"], upstreamPubKey)
		}
	}
	for i := 0; i < 4; i++ {
		ob := outbounds[i].(map[string]any)
		if ob["server"] != "upstream.example.com" {
			t.Errorf("outbound[%d].server = %v, want upstream.example.com", i, ob["server"])
		}
	}

	assertRoutesByAuthUser(t, raw)
}

func TestGenerateConfigUnknownNodeType(t *testing.T) {
	data := &MasterConfigResponse{
		Node: MasterNode{NodeType: "unknown"},
	}

	_, err := GenerateConfig(data, nil)
	if err == nil {
		t.Fatal("expected error for unknown node type, got nil")
	}
}
