package main

import (
	"testing"
)

func TestEncodeQueryStatsRequest(t *testing.T) {
	got := encodeQueryStatsRequest("user>>>", true)
	want := []byte{0x0a, 0x07, 'u', 's', 'e', 'r', '>', '>', '>', 0x10, 0x01}

	if len(got) != len(want) {
		t.Fatalf("encoded length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("encoded[%d] = 0x%x, want 0x%x", i, got[i], want[i])
		}
	}
}

func encodeStat(name string, value int64) []byte {
	stat := []byte{0x0a, byte(len(name))}
	stat = append(stat, name...)
	if value > 0 {
		stat = append(stat, 0x10)
		v := uint64(value)
		for v >= 0x80 {
			stat = append(stat, byte(v)|0x80)
			v >>= 7
		}
		stat = append(stat, byte(v))
	}
	return stat
}

func TestDecodeQueryStatsResponse(t *testing.T) {
	stat1 := encodeStat("user>>>1>>>traffic>>>uplink", 300)
	stat2 := encodeStat("user>>>1>>>traffic>>>downlink", 700)

	var response []byte
	for _, stat := range [][]byte{stat1, stat2} {
		response = append(response, 0x0a, byte(len(stat)))
		response = append(response, stat...)
	}

	stats, err := decodeQueryStatsResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats["user>>>1>>>traffic>>>uplink"] != 300 {
		t.Errorf("uplink = %d, want 300", stats["user>>>1>>>traffic>>>uplink"])
	}
	if stats["user>>>1>>>traffic>>>downlink"] != 700 {
		t.Errorf("downlink = %d, want 700", stats["user>>>1>>>traffic>>>downlink"])
	}
}

func TestDecodeQueryStatsResponseRejectsGarbage(t *testing.T) {
	if _, err := decodeQueryStatsResponse([]byte{0xff, 0x01}); err == nil {
		t.Fatal("expected error for garbage input, got nil")
	}
}

func TestAggregateUserTraffic(t *testing.T) {
	stats := map[string]int64{
		"user>>>1>>>traffic>>>uplink":   300,
		"user>>>1>>>traffic>>>downlink": 700,
		"user>>>2>>>traffic>>>uplink":   50,
		"user>>>bad>>>traffic>>>uplink": 10,
		"inbound>>>x>>>traffic>>>up":    99,
	}

	reports := aggregateUserTraffic(stats)
	if len(reports) != 2 {
		t.Fatalf("got %d reports, want 2", len(reports))
	}

	totals := make(map[int64]int64)
	for _, r := range reports {
		totals[r.TgID] = r.Bytes
	}
	if totals[1] != 1000 {
		t.Errorf("user 1 total = %d, want 1000", totals[1])
	}
	if totals[2] != 50 {
		t.Errorf("user 2 total = %d, want 50", totals[2])
	}
}
