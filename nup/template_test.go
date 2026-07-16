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
