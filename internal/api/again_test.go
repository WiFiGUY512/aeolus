package api

import (
	"strings"
	"testing"
)

// An AP that lost its token enrolls again and waits, as it was, for a person
// to let it back in (0223).
func TestEnrollAgainWaitsToBeLetBackIn(t *testing.T) {
	f := newFixture(t)
	ap, old := f.enrolled()
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "rename", "tree": "locations", "node": ap, "name": "Pump"}); code != 200 {
		t.Fatalf("rename: %d %v", code, b)
	}
	reflashed := map[string]any{}
	for k, v := range shed {
		reflashed[k] = v
	}
	reflashed["openwrt"] = "25.12.6"
	code, _, body := f.apDo("POST", "/v1/enroll", "", reflashed, nil)
	if code != 201 || body["ap"] != ap || body["name"] != "Pump" || body["waiting"] != true {
		t.Fatalf("enrolling again: %d %v", code, body)
	}
	token := body["token"].(string)
	// It polls as one in Landing Zone does, and gets nothing else; its old
	// token still works.
	if code, _, b := f.apDo("GET", "/v1/ap/config", token, nil, nil); code != 200 || b["state"] != "unassigned" || b["waiting"] != true || b["ap"] != ap {
		t.Fatalf("its poll while it waits: %d %v", code, b)
	}
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, reflashed, nil); code != 409 || !strings.Contains(b["error"].(string), "let it back in") {
		t.Fatalf("saying what it is while it waits: %d %v", code, b)
	}
	if code, _, b := f.apDo("GET", "/v1/ap/config", old, nil, nil); code != 200 || b["state"] != "ready" {
		t.Fatalf("its old token while it waits: %d %v", code, b)
	}
	// Its page, the tree and its alerts say so.
	_, page := f.do("GET", "/v1/trees/locations/nodes/"+ap, "griff", nil)
	if again, _ := page["again"].(map[string]any); again["openwrt"] != "25.12.6" {
		t.Fatalf("its page: %v", page["again"])
	}
	_, tree := f.do("GET", "/v1/trees/locations", "griff", nil)
	found := false
	for _, n := range tree["nodes"].([]any) {
		if m := n.(map[string]any); m["id"] == ap {
			found = m["again"] == true
		}
	}
	if !found {
		t.Fatalf("the tree does not say it waits: %v", tree["nodes"])
	}
	_, al := f.do("GET", "/v1/alerts", "griff", nil)
	said := false
	for _, a := range al["alerts"].([]any) {
		if m := a.(map[string]any); m["ap"] == ap && m["kind"] == "again" && m["severity"] == "critical" && strings.Contains(m["message"].(string), "127.0.0.1") {
			said = true
		}
	}
	if !said {
		t.Fatalf("no alert that it waits: %v", al["alerts"])
	}
	// Let back in, it is as it was, with its new token.
	if code, b := f.change("griff", map[string]any{"kind": "readmit-ap", "node": ap}); code != 200 {
		t.Fatalf("readmit-ap: %d %v", code, b)
	}
	if code, _, b := f.apDo("GET", "/v1/ap/config", token, nil, nil); code != 200 || b["state"] != "ready" || b["ap"] != ap {
		t.Fatalf("its poll, let back in: %d %v", code, b)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/config", old, nil, nil); code != 401 {
		t.Fatalf("its old token, once let back in: %d", code)
	}
	_, page = f.do("GET", "/v1/trees/locations/nodes/"+ap, "griff", nil)
	if node := page["node"].(map[string]any); node["name"] != "Pump" || node["parent"] != "office" || page["again"] != nil || page["facts"].(map[string]any)["openwrt"] != "25.12.6" {
		t.Fatalf("its page, let back in: %v", page)
	}
}

// Another AP, from Landing Zone, takes an AP's place and keeps its records
// (0223); the box it replaced is a stranger after.
func TestReplaceAP(t *testing.T) {
	f := newFixture(t)
	ap, old := f.enrolled()
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, b)
	}
	box := map[string]any{"mac": "02:00:00:00:aa:01", "hostname": "OpenWrt", "model": "Arista C-260", "board": "arista,c-260", "openwrt": "SNAPSHOT"}
	code, _, body := f.apDo("POST", "/v1/enroll", "", box, nil)
	if code != 201 || body["ap"] != "ap-02000000aa01" {
		t.Fatalf("the new box: %d %v", code, body)
	}
	token := body["token"].(string)
	if code, b := f.change("griff", map[string]any{"kind": "replace-ap", "node": ap, "with": "ap-02000000aa01"}); code != 200 {
		t.Fatalf("replace-ap: %d %v", code, b)
	}
	if code, _, b := f.apDo("GET", "/v1/ap/config", token, nil, nil); code != 200 || b["state"] != "ready" || b["ap"] != ap {
		t.Fatalf("the new box's poll: %d %v", code, b)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/config", old, nil, nil); code != 401 {
		t.Fatalf("the old box's token: %d", code)
	}
	// It says what it is by its own MAC, which the AP is known by now.
	box["openwrt"] = "25.12.6"
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, box, nil); code != 200 || b["changed"] != true {
		t.Fatalf("its facts: %d %v", code, b)
	}
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, shed, nil); code != 400 {
		t.Fatalf("its facts, by the old box's MAC: %d %v", code, b)
	}
	// The new box, reflashed, is known as this AP.
	code, _, body = f.apDo("POST", "/v1/enroll", "", box, nil)
	if code != 201 || body["ap"] != ap || body["waiting"] != true {
		t.Fatalf("the new box, enrolling again: %d %v", code, body)
	}
	// The old box, back, is a new AP by the next ID free: its own is this
	// AP's now.
	code, _, body = f.apDo("POST", "/v1/enroll", "", shed, nil)
	if code != 201 || body["ap"] != ap+"-2" || body["waiting"] != nil {
		t.Fatalf("the old box, back: %d %v", code, body)
	}
}
