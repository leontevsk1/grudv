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

func decodeConfig(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("GenerateConfig produced invalid JSON: %v", err)
	}
	return config
}

func decodeInbounds(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	config := decodeConfig(t, raw)
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

func routeRules(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	config := decodeConfig(t, raw)
	rulesAny := config["route"].(map[string]any)["rules"].([]any)
	rules := make([]map[string]any, len(rulesAny))
	for i, r := range rulesAny {
		rules[i] = r.(map[string]any)
	}
	return rules
}

func TestGenerateConfigReality(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "reality"},
		Users: testUsers(),
	}

	raw, err := GenerateConfig(data)
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

	if name := vlessUsers[0].(map[string]any)["name"]; name != "1" {
		t.Errorf("vless user name = %v, want tg_id \"1\"", name)
	}

	rules := routeRules(t, raw)
	if len(rules) != 3 {
		t.Fatalf("got %d route rules, want 3 (private-block, premium, free)", len(rules))
	}
	premiumRule := rules[1]["user"].([]any)
	if len(premiumRule) != 1 || premiumRule[0] != "1" {
		t.Errorf("premium rule users = %v, want [\"1\"]", premiumRule)
	}
	freeRule := rules[2]["user"].([]any)
	if len(freeRule) != 1 || freeRule[0] != "2" {
		t.Errorf("free rule users = %v, want [\"2\"]", freeRule)
	}

	config := decodeConfig(t, raw)
	statsUsers := config["experimental"].(map[string]any)["v2ray_api"].(map[string]any)["stats"].(map[string]any)["users"].([]any)
	if len(statsUsers) != 2 {
		t.Errorf("v2ray_api stats users = %d, want 2", len(statsUsers))
	}
}

func TestGenerateConfigOmitsRuleForEmptyTier(t *testing.T) {
	data := &MasterConfigResponse{
		Node: MasterNode{NodeType: "reality"},
		Users: []MasterUser{
			{TgID: 1, Tier: "premium", VlessUUID: strPtr("uuid-premium")},
		},
	}

	raw, err := GenerateConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rules := routeRules(t, raw)
	if len(rules) != 2 {
		t.Fatalf("got %d route rules, want 2 (private-block, premium; no empty free matcher)", len(rules))
	}
}

func TestGenerateConfigWeb(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "web"},
		Users: testUsers(),
	}

	raw, err := GenerateConfig(data)
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
}

func TestGenerateConfigRelayRequiresUpstream(t *testing.T) {
	data := &MasterConfigResponse{
		Node:         MasterNode{NodeType: "relay"},
		Users:        testUsers(),
		UpstreamNode: nil,
	}

	_, err := GenerateConfig(data)
	if err == nil {
		t.Fatal("expected error when relay node has no upstream, got nil")
	}
}

func TestGenerateConfigRelayWithUpstream(t *testing.T) {
	data := &MasterConfigResponse{
		Node:  MasterNode{NodeType: "relay"},
		Users: testUsers(),
		UpstreamNode: &MasterNode{
			Address: "upstream.example.com",
		},
	}

	raw, err := GenerateConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("GenerateConfig produced invalid JSON: %v", err)
	}

	outbounds := config["outbounds"].([]any)
	for i := 0; i < 4; i++ {
		ob := outbounds[i].(map[string]any)
		if ob["server"] != "upstream.example.com" {
			t.Errorf("outbound[%d].server = %v, want upstream.example.com", i, ob["server"])
		}
	}

	rules := routeRules(t, raw)
	if len(rules) != 5 {
		t.Fatalf("got %d route rules, want 5 (private-block + 2 tcp + 2 udp)", len(rules))
	}
	tcpPremium := rules[1]
	if tcpPremium["outbound"] != "Upstream-TCP-Premium" {
		t.Errorf("rule[1].outbound = %v, want Upstream-TCP-Premium", tcpPremium["outbound"])
	}
	if inbound := tcpPremium["inbound"].([]any); inbound[0] != "in-vless-reality" {
		t.Errorf("rule[1].inbound = %v, want [in-vless-reality]", inbound)
	}
}

func TestGenerateConfigUnknownNodeType(t *testing.T) {
	data := &MasterConfigResponse{
		Node: MasterNode{NodeType: "unknown"},
	}

	_, err := GenerateConfig(data)
	if err == nil {
		t.Fatal("expected error for unknown node type, got nil")
	}
}
