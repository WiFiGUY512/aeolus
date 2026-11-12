package api

import (
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What an AP's Bluetooth radio hears, device by device (0157): kept with
// its report, each device named and said what it is by the manager before
// it is stored, the rotating addresses counted; and a device or group said
// badly left out, the report kept (0241).
func TestBluetoothDevices(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	post := func(heard map[string]any) int {
		code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "ble_devices": heard}, nil)
		return code
	}
	heard := func() map[string]any {
		_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
		b, _ := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["ble_devices"].(map[string]any)
		return b
	}
	devices := []any{
		// A sensor with a name, from a static random address.
		map[string]any{"mac": "c1:00:00:00:00:01", "type": "random", "random": "static", "rssi": -58, "rssi_avg": -60, "rssi_max": -55,
			"seen": 67, "first_ago": 300, "last_ago": 2, "company": 0x0969, "mfr": "de64ec95", "uuid16": []any{"fd3d"}},
		// An AirTag.
		map[string]any{"mac": "c2:00:00:00:00:02", "type": "random", "random": "static", "rssi": -68, "seen": 8, "first_ago": 400, "last_ago": 4,
			"company": 0x004c, "mfr": "1219", "tracker": "apple_find_my"},
		// One from a public address: its maker by its OUI. The AP's own
		// words for what the manager adds are not taken.
		map[string]any{"mac": "30:86:2d:00:00:03", "type": "public", "rssi": -66, "seen": 129, "first_ago": 600, "last_ago": 2, "name": "A battery",
			"maker": "made up", "kind": "made up"},
		// An iBeacon.
		map[string]any{"mac": "c3:00:00:00:00:04", "type": "random", "random": "static", "rssi": -70, "seen": 3, "first_ago": 30, "last_ago": 3,
			"company": 0x004c, "mfr": "0215", "beacon": map[string]any{"type": "ibeacon", "uuid": "01020304-0506-0708-090a-0b0c0d0e0f10", "major": 7, "minor": 300, "tx_1m": -59}},
	}
	rotating := []any{map[string]any{"company": 6, "mfr": "010920", "count": 4, "rssi_max": -57, "last_ago": 2}}
	if code := post(map[string]any{"source": "both", "surveyed_ago": 41, "devices": devices, "rotating": rotating}); code != 200 {
		t.Fatalf("what the radio hears: %d", code)
	}
	b := heard()
	if b["source"] != "both" || b["surveyed_ago"] != float64(41) {
		t.Fatalf("ble_devices = %v", b)
	}
	got := map[string]map[string]any{}
	for _, d := range b["devices"].([]any) {
		got[d.(map[string]any)["mac"].(string)] = d.(map[string]any)
	}
	for mac, want := range map[string][3]string{
		"c1:00:00:00:00:01": {"", "SwitchBot", "Woan Technology (Shenzhen) Co., Ltd."},
		"c2:00:00:00:00:02": {"tracker", "Apple Find My", "Apple, Inc."},
		"30:86:2d:00:00:03": {"", "", ""},
		"c3:00:00:00:00:04": {"beacon", "iBeacon", "Apple, Inc."},
	} {
		d := got[mac]
		if d == nil || str(d["kind"]) != want[0] || str(d["what"]) != want[1] || str(d["company_name"]) != want[2] {
			t.Errorf("%s: %v; want kind %q, what %q, company %q", mac, d, want[0], want[1], want[2])
		}
	}
	if d := got["30:86:2d:00:00:03"]; d["maker"] != "Arista Network, Inc." || d["name"] != "A battery" {
		t.Errorf("the public one: %v", d)
	}
	if s := got["c1:00:00:00:00:01"]["services"]; s == nil || s.([]any)[0] != "Woan Technology (Shenzhen) Co., Ltd." {
		t.Errorf("its services: %v", s)
	}
	g := b["rotating"].([]any)[0].(map[string]any)
	if g["count"] != float64(4) || g["kind"] != "personal" || g["what"] != "Windows device" || g["company_name"] != "Microsoft" {
		t.Errorf("rotating: %v", g)
	}

	// Kept (0158): each device the AP heard, what each AP heard of it, and
	// the rotating addresses by what they are.
	code, h := f.do("GET", "/v1/bluetooth?under="+ap+"&hours=24", "griff", nil)
	if code != 200 {
		t.Fatalf("GET /v1/bluetooth: %d %v", code, h)
	}
	if n := len(h["devices"].([]any)); n != 4 {
		t.Errorf("devices kept: %d", n)
	}
	kept := map[string]map[string]any{}
	for _, d := range h["devices"].([]any) {
		kept[d.(map[string]any)["mac"].(string)] = d.(map[string]any)
	}
	if d := kept["c2:00:00:00:00:02"]; d == nil || d["kind"] != "tracker" || d["last_ap"] != ap || len(d["aps"].([]any)) != 1 {
		t.Errorf("the tracker, kept: %v", d)
	}
	if r := h["rotating"].([]any); len(r) != 1 || r[0].(map[string]any)["what"] != "Microsoft · Windows device" || r[0].(map[string]any)["count"] != float64(4) {
		t.Errorf("rotating, kept: %v", r)
	}
	code, history := f.do("GET", "/v1/bluetooth/devices/c1:00:00:00:00:01", "griff", nil)
	if code != 200 || history["device"].(map[string]any)["what"] != "SwitchBot" || len(history["seen"].([]any)) != 1 ||
		history["seen"].([]any)[0].(map[string]any)["rssi"] != float64(-58) {
		t.Errorf("one device's history: %d %v", code, history)
	}
	if code, _ := f.do("GET", "/v1/bluetooth/devices/c9:00:00:00:00:09", "griff", nil); code != 404 {
		t.Errorf("one never heard: %d", code)
	}
	if code, _ := f.do("GET", "/v1/bluetooth/devices/nope", "griff", nil); code != 400 {
		t.Errorf("no MAC: %d", code)
	}
	if code, _ := f.do("GET", "/v1/bluetooth?hours=0", "griff", nil); code != 400 {
		t.Errorf("no hours: %d", code)
	}

	// Said badly: left out, the rest kept with the report (0241). The
	// Living Room's fridge once said it was heard at 99 dBm, and every
	// report it sent was refused for it.
	good := map[string]any{"mac": "c9:00:00:00:00:09", "type": "random", "random": "static", "rssi": -60, "seen": 1, "first_ago": 1, "last_ago": 1}
	one := func(change map[string]any) map[string]any {
		d := map[string]any{"mac": "c1:00:00:00:00:01", "type": "random", "seen": 1, "first_ago": 1, "last_ago": 1}
		for k, v := range change {
			d[k] = v
		}
		return map[string]any{"source": "survey", "devices": []any{d, good}, "rotating": []any{}}
	}
	for name, said := range map[string]map[string]any{
		"a MAC that is none":        one(map[string]any{"mac": "nope"}),
		"a signal past the top":     one(map[string]any{"rssi": 30}),
		"its strongest at 99 dBm":   one(map[string]any{"rssi": -68, "rssi_max": 99}),
		"a name with a control":     one(map[string]any{"name": "a\x07b"}),
		"a name too long":           one(map[string]any{"name": strings.Repeat("n", 65)}),
		"data not hex":              one(map[string]any{"mfr": "xyz"}),
		"a UUID that is none":       one(map[string]any{"uuid16": []any{"12345"}}),
		"a beacon of no known kind": one(map[string]any{"beacon": map[string]any{"type": "other"}}),
		"a group of none": {"source": "proxy", "devices": []any{good},
			"rotating": []any{map[string]any{"count": 0, "last_ago": 1}, map[string]any{"company": 6, "count": 2, "last_ago": 1}}},
	} {
		if code := post(said); code != 200 {
			t.Errorf("%s: %d", name, code)
			continue
		}
		b := heard()
		ds, _ := b["devices"].([]any)
		if len(ds) != 1 || ds[0].(map[string]any)["mac"] != "c9:00:00:00:00:09" {
			t.Errorf("%s: devices %v", name, ds)
		}
		if gs, _ := b["rotating"].([]any); name == "a group of none" && (len(gs) != 1 || gs[0].(map[string]any)["count"] != float64(2)) {
			t.Errorf("%s: rotating %v", name, gs)
		}
	}
	// With no source it says nothing that can be kept, and the report is
	// kept without it.
	if code := post(map[string]any{"devices": []any{good}, "rotating": []any{}}); code != 200 || heard() != nil {
		t.Errorf("no source: %d, %v", code, heard())
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// A Bluetooth device a person names is called so (0220); one named and
// not heard for a day is an alert on the AP that heard it last, until it
// has been gone a month.
func TestBluetoothNames(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	now := time.Now()
	heard := func(mac string, ago time.Duration) conditions.BLEHeard {
		return conditions.BLEHeard{MAC: mac, At: now.Add(-ago), First: now.Add(-ago - time.Hour), RSSI: -60, Type: "random"}
	}
	if err := f.api.conds.RecordBluetooth(id, []conditions.BLEHeard{heard("c1:00:00:00:00:01", time.Minute), heard("c5:00:00:00:00:05", 25*time.Hour), heard("c6:00:00:00:00:06", 40*24*time.Hour)}, nil); err != nil {
		t.Fatal(err)
	}
	name := func(mac, n string) (int, map[string]any) {
		return f.do("PUT", "/v1/bluetooth/devices/"+mac+"/name", "griff", map[string]any{"name": n})
	}
	for mac, n := range map[string]string{"c1:00:00:00:00:01": "  attic   sensor ", "c5:00:00:00:00:05": "garage sensor", "c6:00:00:00:00:06": "old tag"} {
		if code, b := name(mac, n); code != 200 {
			t.Fatalf("name %s: %d %v", mac, code, b)
		}
	}
	if code, _ := name("c9:00:00:00:00:09", "nothing heard"); code != 404 {
		t.Fatalf("a device never heard: %d", code)
	}
	if code, _ := name("nope", "x"); code != 400 {
		t.Fatalf("no MAC: %d", code)
	}
	if code, _ := name("c1:00:00:00:00:01", "bell\x07"); code != 400 {
		t.Fatalf("a name with a control character: %d", code)
	}
	_, list := f.do("GET", "/v1/bluetooth?hours=2160", "griff", nil)
	named := map[string]any{}
	for _, d := range list["devices"].([]any) {
		m := d.(map[string]any)
		named[m["mac"].(string)] = m["named"]
	}
	if named["c1:00:00:00:00:01"] != "attic sensor" || named["c5:00:00:00:00:05"] != "garage sensor" {
		t.Fatalf("names = %v", named)
	}
	_, one := f.do("GET", "/v1/bluetooth/devices/c1:00:00:00:00:01?hours=2160", "griff", nil)
	if one["device"].(map[string]any)["named"] != "attic sensor" {
		t.Fatalf("one device = %v", one["device"])
	}

	// The garage sensor, unheard for 25 hours, is an alert; the attic
	// sensor heard a minute ago is not, nor the tag gone forty days.
	silent := f.api.bleSilent(now)[id]
	if len(silent) != 1 || silent[0].MAC != "c5:00:00:00:00:05" || silent[0].Name != "garage sensor" {
		t.Fatalf("silent = %+v", silent)
	}
	_, al := f.do("GET", "/v1/alerts", "griff", nil)
	var keys []string
	for _, a := range al["alerts"].([]any) {
		if m := a.(map[string]any); m["kind"] == "ble-silent" {
			keys = append(keys, m["severity"].(string)+":"+m["key"].(string))
		}
	}
	if strings.Join(keys, " ") != "info:ble-silent:c5:00:00:00:00:05" {
		t.Fatalf("alerts = %v", keys)
	}
	// Its name taken away, it is no one's to miss.
	if code, _ := name("c5:00:00:00:00:05", ""); code != 200 {
		t.Fatalf("unname: %d", code)
	}
	if silent := f.api.bleSilent(now)[id]; len(silent) != 0 {
		t.Fatalf("unnamed, still silent: %+v", silent)
	}
}
