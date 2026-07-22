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

func TestGenerateConfigReality(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "reality"},
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

func TestGenerateConfigWeb(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "web"},
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
