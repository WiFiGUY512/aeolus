package api

import (
	"strings"
	"testing"
)

// A state report says the AP's radios whose firmware crashed (0310): each
// as the agent writes it is taken, one not is left out and the report taken
// all the same (0290), and the AP's alerts say it.
func TestReportFWCrashes(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := map[string]any{"version": version, "uptime": 4000, "fw_crashes": []any{
		map[string]any{"ago": 300, "driver": "ath11k_pci", "device": "0001:01:00.0", "why": "MHI_CB_EE_RDDM", "recovered": false, "lost": true, "radios": []string{"radio2"}, "bands": []string{}},
		map[string]any{"ago": -5, "driver": "ath11k", "device": "c000000.wifi", "recovered": true},
		map[string]any{"ago": 60, "driver": "ath11k", "device": "has a space", "recovered": true},
	}}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	_, view := f.do("GET", "/v1/alerts", "griff", nil)
	var found []string
	for _, a := range view["alerts"].([]any) {
		if m := a.(map[string]any); m["kind"] == "fw-crash" && m["ap"] == ap {
			found = append(found, m["severity"].(string)+" "+m["message"].(string))
		}
	}
	if len(found) != 1 || !strings.HasPrefix(found[0], "critical the firmware of a radio (ath11k_pci 0001:01:00.0) crashed (MHI_CB_EE_RDDM) and it did not come back") {
		t.Fatalf("fw-crash alerts: %v", found)
	}
}
