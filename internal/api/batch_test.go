package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A batch is several sets and unsets on nodes of either tree made as one
// change (0203): each readied, checked and allowed as if made alone, all of
// them made or none, its secrets sealed and never shown.
func TestBatch(t *testing.T) {
	f := newFixture(t)
	const newPass = "batch-passphrase-9"
	batch := func(ops ...map[string]any) map[string]any {
		return map[string]any{"kind": "batch", "ops": ops}
	}
	set := func(tree, node, path string, v any) map[string]any {
		return map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v}
	}
	unset := func(tree, node, path string) map[string]any {
		return map[string]any{"kind": "unset", "tree": tree, "node": node, "path": path}
	}

	// A preview says what each change would leave its fields at.
	code, body := f.do("POST", "/v1/preview", "griff", map[string]any{"op": batch(
		set("services", "household", "network.sweet.passphrase", newPass),
		unset("locations", "house", "radio.5g.width"),
	)})
	if code != 200 {
		t.Fatalf("preview: %d %v", code, body)
	}
	resolved := body["resolved"].([]any)
	if len(resolved) != 2 || resolved[1].(map[string]any)["node"] != "house" {
		t.Fatalf("resolved = %v", resolved)
	}
	if raw, _ := json.Marshal(body); strings.Contains(string(raw), newPass) {
		t.Fatalf("the preview shows the passphrase: %s", raw)
	}

	seq := f.log.Seq()
	code, body = f.change("griff", batch(
		set("services", "household", "network.sweet.passphrase", newPass),
		unset("locations", "house", "radio.5g.width"),
	))
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if f.log.Seq() != seq+1 {
		t.Fatalf("seq %d after %d: a batch is one change", f.log.Seq(), seq)
	}
	op := body["change"].(map[string]any)["op"].(map[string]any)
	if v := op["ops"].([]any)[0].(map[string]any)["value"].(map[string]any); v["sealed"] != true {
		t.Fatalf("returned passphrase = %v", v)
	}
	if rv := body["reversioned"].([]any); len(rv) != 1 || rv[0] != "office-ap" {
		t.Fatalf("reversioned = %v", rv)
	}
	if v, _ := f.log.Snapshot().Org.Locations.Own("house", "radio.5g.width"); v != nil {
		t.Fatalf("width = %v, want unset", v)
	}
	code, b := f.do("GET", "/v1/changes?limit=1000", "claude", nil)
	if raw, _ := json.Marshal(b); code != 200 || strings.Contains(string(raw), newPass) {
		t.Fatalf("changes: %d, or it shows the passphrase", code)
	}

	// One that cannot be made, under a lock, and none is.
	seq = f.log.Seq()
	code, body = f.change("claude", batch(
		set("locations", "house", "radio.5g.width", 20),
		set("locations", "house", "system.poll", 30),
	))
	if code != 409 || !strings.Contains(body["error"].(string), "change 2 of 2") {
		t.Fatalf("under a lock: %d %v", code, body)
	}
	if v, _ := f.log.Snapshot().Org.Locations.Own("house", "radio.5g.width"); v != nil || f.log.Seq() != seq {
		t.Fatalf("width = %v, seq %d after %d: the batch was partly made", v, f.log.Seq(), seq)
	}

	cases := []struct {
		name string
		as   string
		op   map[string]any
		code int
		msg  string
	}{
		{"each change is allowed alone", "office", batch(set("services", "household", "network.sweet.ssid", "Sweet"), set("locations", "house", "radio.5g.width", 20)), 403, "needs configure"},
		{"each change is checked alone", "griff", batch(unset("services", "household", "radio.5g.width")), 400, "other tree"},
		{"each value is checked", "griff", batch(set("locations", "house", "radio.5g.width", 33)), 400, "value must be one of"},
		{"a lock is no batch's", "griff", batch(map[string]any{"kind": "lock", "tree": "locations", "node": "house", "path": "system.tz"}), 400, "which a batch does not make"},
		{"at least one", "griff", batch(), 400, "1 to 256"},
	}
	for _, c := range cases {
		code, body := f.change(c.as, c.op)
		if code != c.code || !strings.Contains(body["error"].(string), c.msg) {
			t.Errorf("%s: %d %v, want %d containing %q", c.name, code, body, c.code, c.msg)
		}
	}

	must(t, f.log.Close())
	files, _ := filepath.Glob(filepath.Join(f.dir, "aeolus.db*"))
	for _, file := range files {
		data, err := os.ReadFile(file)
		must(t, err)
		if strings.Contains(string(data), newPass) {
			t.Fatalf("%s contains the passphrase", filepath.Base(file))
		}
	}
}

// A page can make a services folder, have a location take it and put a
// network in it, as one change (0320); and a Services folder's page says,
// of each Locations folder, which service folders it takes and from where.
func TestBatchANewServicesFolder(t *testing.T) {
	f := newFixture(t)
	op := map[string]any{"kind": "batch", "ops": []map[string]any{
		{"kind": "add-folder", "tree": "services", "node": "guests", "name": "Guests", "parent": "example"},
		{"kind": "assign-services", "node": "house", "services": []string{"household", "guests"}},
		{"kind": "set", "tree": "services", "node": "guests", "values": map[string]any{
			"network.visitor.ssid": "Visitor", "network.visitor.security": "open",
			"network.visitor.transport.primary.type": "vlan", "network.visitor.transport.primary.vlan": 30,
		}},
	}}
	if code, body := f.do("POST", "/v1/preview", "griff", map[string]any{"op": op}); code != 200 {
		t.Fatalf("preview: %d %v", code, body)
	}
	seq := f.log.Seq()
	code, body := f.change("griff", op)
	if code != 200 {
		t.Fatalf("change: %d %v", code, body)
	}
	if f.log.Seq() != seq+1 {
		t.Fatalf("seq %d after %d: one change", f.log.Seq(), seq)
	}
	if rv := body["reversioned"].([]any); len(rv) != 1 || rv[0] != "office-ap" {
		t.Fatalf("reversioned = %v", rv)
	}

	// Its page: the Org takes household, set there; House takes both, set
	// at House; Office, below it, as House.
	code, page := f.do("GET", "/v1/trees/services/nodes/guests", "griff", nil)
	if code != 200 {
		t.Fatalf("page: %d %v", code, page)
	}
	got := map[string]string{}
	for _, x := range page["placements"].([]any) {
		p := x.(map[string]any)
		var names []string
		for _, s := range p["services"].([]any) {
			names = append(names, s.(string))
		}
		got[p["node"].(string)] = strings.Join(names, "+") + " from " + fmt.Sprint(p["from"])
	}
	want := map[string]string{"example": "household from example", "house": "household+guests from house", "office": "household+guests from house"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("placement %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if _, ok := got["office-ap"]; ok {
		t.Error("an AP is a placement")
	}

	// A change of another kind is still no batch's.
	code, body = f.change("griff", map[string]any{"kind": "batch", "ops": []map[string]any{{"kind": "move", "tree": "services", "node": "guests", "parent": "household"}}})
	if code != 400 {
		t.Fatalf("a move in a batch: %d %v", code, body)
	}
}
