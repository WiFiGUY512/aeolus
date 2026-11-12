package api

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A pcap file as tcpdump writes one: its header, then packets, each with
// its time in seconds and microseconds and its bytes.
func pcapClassic(link uint32, packets ...[3]int) []byte {
	le := binary.LittleEndian
	b := le.AppendUint32(nil, 0xa1b2c3d4)
	b = append(b, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	b = le.AppendUint32(b, 128)
	b = le.AppendUint32(b, link)
	for _, p := range packets {
		b = le.AppendUint32(b, uint32(p[0]))
		b = le.AppendUint32(b, uint32(p[1]))
		b = le.AppendUint32(b, uint32(p[2]))
		b = le.AppendUint32(b, uint32(p[2]+900))
		b = append(b, bytes.Repeat([]byte{byte(p[1] / 100)}, p[2])...)
	}
	return b
}

// A capture someone starts (0148): an admin of the AP asks for one (0177), of
// a network or of a VLAN; the AP is handed it, sends it up in pieces, and
// is told when someone stops it; and it is read as one file, its
// interfaces' packets together by time.
func TestCapturesPeopleStart(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	start := func(as string, ask map[string]any) (int, map[string]any) {
		t.Helper()
		return f.do("POST", "/v1/aps/"+ap+"/pcaps", as, ask)
	}
	// One with no role on the AP is told nothing; any role but admin is
	// refused: packets are an admin's (0177).
	if code, _ := start("tenant", map[string]any{"sides": "both", "network": "sweet"}); code != 404 {
		t.Fatalf("one with no role on the AP asking: %d", code)
	}
	for _, role := range []string{"viewer", "operator", "manager", "installer", "admin"} {
		if code, b := f.change("griff", map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": role}); code != 200 {
			t.Fatalf("grant: %d %v", code, b)
		}
		if role == "admin" {
			break
		}
		if code, b := start("tenant", map[string]any{"sides": "both", "network": "sweet"}); code != 403 {
			t.Errorf("%s asking: %d %v", role, code, b)
		}
	}
	for what, ask := range map[string]map[string]any{
		"no sides":                  {"network": "sweet"},
		"sides that are none":       {"sides": "air", "network": "sweet"},
		"neither network nor VLAN":  {"sides": "both"},
		"both a network and a VLAN": {"sides": "both", "network": "sweet", "vlan": 10},
		"a network the AP has not":  {"sides": "both", "network": "nowhere"},
		"a VLAN there is none of":   {"sides": "wired", "vlan": 5000},
		"a client that is no MAC":   {"sides": "both", "network": "sweet", "client": "phone"},
		"a band there is none of":   {"sides": "wireless", "network": "sweet", "bands": []string{"60g"}},
		"too short":                 {"sides": "both", "network": "sweet", "seconds": 2},
		"too long":                  {"sides": "both", "network": "sweet", "seconds": 3600},
	} {
		if code, body := start("tenant", ask); code != 400 {
			t.Errorf("%s: %d %v", what, code, body)
		}
	}
	const mac = "72:bb:66:9f:76:94"
	// The AP holds its line to the manager (0145): it is told of the
	// capture at once, with what to take, and does not wait for its poll.
	lines, hang := f.lineOf("/v1/ap/live", token)
	defer hang()
	next(t, lines, "as the line opens", func(map[string]any) bool { return true })
	code, body := start("tenant", map[string]any{"sides": "both", "network": "sweet", "client": "72:BB:66:9F:76:94", "bands": []string{"5g", "5g"}})
	if code != 200 {
		t.Fatalf("asking for one: %d %v", code, body)
	}
	k := body["capture"].(map[string]any)
	said, took := next(t, lines, "the capture, on the AP's line", func(v map[string]any) bool { return v["capture"] != nil })
	if c := said["capture"].(map[string]any); c["id"] != k["id"] || c["params"].(map[string]any)["network"] != "sweet" || c["params"].(map[string]any)["client"] != mac || said["now"] == nil {
		t.Errorf("what the AP is told on its line: %v", said)
	}
	if took > 200*time.Millisecond {
		t.Errorf("the AP was told %v after it was asked", took)
	}
	if k["told"] != true {
		t.Errorf("the capture does not say its AP was told: %v", k)
	}
	id := int64(k["id"].(float64))
	ask := k["ask"].(map[string]any)
	if k["state"] != "pending" || k["actor"] != "tenant" || k["ap_name"] != "ShedAP" || ask["sides"] != "both" || ask["client"] != mac || ask["ssid"] == "" ||
		ask["snap"] != float64(128) || ask["seconds"] != float64(60) || ask["bytes"] != float64(20<<20) || fmt.Sprint(ask["bands"]) != "[5g]" {
		t.Errorf("the capture asked for: %v", k)
	}
	// Asked again the same, it is the one that waits; another way, it is refused.
	if code, body := start("griff", map[string]any{"sides": "both", "network": "sweet", "client": mac, "bands": []string{"5g"}}); code != 200 || int64(body["capture"].(map[string]any)["id"].(float64)) != id {
		t.Errorf("asked again: %d %v", code, body)
	}
	if code, body := start("griff", map[string]any{"sides": "wired", "network": "sweet", "client": mac}); code != 409 {
		t.Errorf("asked another way while one waits: %d %v", code, body)
	}

	// The AP is handed it, with what to take.
	_, _, got := f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	acts := got["actions"].([]any)
	if len(acts) != 1 || acts[0].(map[string]any)["kind"] != "capture" || acts[0].(map[string]any)["params"].(map[string]any)["network"] != "sweet" {
		t.Fatalf("what the AP is handed: %v", got)
	}
	// It sends two interfaces' files, in pieces.
	air := pcapClassic(1, [3]int{100, 100, 60}, [3]int{100, 300, 40})
	wire := pcapClassic(1, [3]int{100, 200, 64}, [3]int{100, 400, 44})
	piece := func(n int, name, side, note string, offset int, data []byte) (int, map[string]any) {
		t.Helper()
		code, _, body := f.apDo("POST", fmt.Sprintf("/v1/ap/pcaps/%d", id), token, map[string]any{
			"n": n, "name": name, "side": side, "note": note, "offset": offset, "data": base64.StdEncoding.EncodeToString(data)}, nil)
		return code, body
	}
	if code, body := piece(0, "phy0-ap1", "wireless", "Home Wi-Fi, 5 GHz", 0, air[:50]); code != 200 || body["bytes"] != float64(50) || body["stop"] != false {
		t.Fatalf("the first piece: %d %v", code, body)
	}
	// With a piece, or with none, the AP says how long it took to begin
	// after it heard: kept once, the first said.
	for _, ms := range []int{140, 900} {
		if code, _, body := f.apDo("POST", fmt.Sprintf("/v1/ap/pcaps/%d", id), token, map[string]any{"took": ms}, nil); code != 200 {
			t.Fatalf("saying how long it took to begin: %d %v", code, body)
		}
	}
	// Sent again, it is not kept twice; one that leaves a gap is refused,
	// with where the manager is.
	if code, body := piece(0, "phy0-ap1", "wireless", "Home Wi-Fi, 5 GHz", 0, air[:50]); code != 200 || body["bytes"] != float64(50) {
		t.Errorf("a piece sent again: %d %v", code, body)
	}
	if code, body := piece(0, "phy0-ap1", "wireless", "Home Wi-Fi, 5 GHz", 60, air[60:]); code != 409 || body["bytes"] != float64(50) {
		t.Errorf("a piece that leaves a gap: %d %v", code, body)
	}
	if code, body := piece(0, "phy0-ap1", "wireless", "Home Wi-Fi, 5 GHz", 50, air[50:]); code != 200 || body["bytes"] != float64(len(air)) {
		t.Fatalf("the rest of it: %d %v", code, body)
	}
	if code, body := piece(1, "eth0", "wired", "VLAN 20", 0, wire); code != 200 || body["bytes"] != float64(len(wire)) {
		t.Fatalf("the wired side's: %d %v", code, body)
	}
	for what, bad := range map[string][]any{
		"a side there is none of": {2, "eth1", "air", "", 0},
		"no name":                 {2, "", "wired", "", 0},
		"too many interfaces":     {99, "eth9", "wired", "", 0},
	} {
		if code, body := piece(bad[0].(int), bad[1].(string), bad[2].(string), bad[3].(string), bad[4].(int), nil); code != 400 {
			t.Errorf("%s: %d %v", what, code, body)
		}
	}
	// A capture there is none of is not this AP's to send to.
	if code, _, body := f.apDo("POST", fmt.Sprintf("/v1/ap/pcaps/%d", id+999), token, map[string]any{"n": 0, "name": "x", "side": "wired", "offset": 0, "data": ""}, nil); code != 404 {
		t.Errorf("sending to a capture there is none of: %d %v", code, body)
	}

	// Someone stops it: the AP is told when it next asks, with or without a piece.
	if code, body := f.do("POST", fmt.Sprintf("/v1/pcaps/%d/stop", id), "tenant", nil); code != 200 || body["capture"].(map[string]any)["stopping"] != true {
		t.Fatalf("stopping it: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", fmt.Sprintf("/v1/ap/pcaps/%d", id), token, map[string]any{}, nil); code != 200 || body["stop"] != true {
		t.Errorf("the AP asking whether to go on: %d %v", code, body)
	}
	// While it runs it may be read, and not deleted.
	if code, body := f.do("DELETE", fmt.Sprintf("/v1/pcaps/%d", id), "griff", nil); code != 409 {
		t.Errorf("deleting one that runs: %d %v", code, body)
	}
	// The AP says what came of it.
	if code, _, body := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", id), token, map[string]any{"ok": true, "result": "4 packets in 12 s; stopped",
		"detail": map[string]any{"packets": 4, "why": "stopped", "missed": []string{"phy1-ap1: no such interface"}}}, nil); code != 200 {
		t.Fatalf("the AP saying it is done: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", fmt.Sprintf("/v1/ap/pcaps/%d", id), token, map[string]any{"n": 1, "name": "eth0", "side": "wired", "offset": len(wire), "data": "AAAA"}, nil); code != 404 {
		t.Errorf("a piece after it ended: %d %v", code, body)
	}

	// It is listed for those who may view its AP, and for no one else.
	list := func(as, query string) []any {
		t.Helper()
		code, got := f.do("GET", "/v1/pcaps"+query, as, nil)
		if code != 200 {
			t.Fatalf("%s listing them: %d %v", as, code, got)
		}
		return got["captures"].([]any)
	}
	got1 := list("tenant", "")
	if len(got1) != 1 {
		t.Fatalf("the captures a viewer sees: %v", got1)
	}
	k = got1[0].(map[string]any)
	files := k["files"].([]any)
	if k["state"] != "done" || k["result"] != "4 packets in 12 s; stopped" || len(files) != 2 || k["bytes"] != float64(len(air)+len(wire)) ||
		files[0].(map[string]any)["name"] != "phy0-ap1" || files[1].(map[string]any)["note"] != "VLAN 20" || k["detail"].(map[string]any)["packets"] != float64(4) ||
		len(k["place"].([]any)) == 0 || k["stopping"] != nil || k["told"] != true || k["took_ms"] != float64(140) {
		t.Errorf("the capture, listed: %v", k)
	}
	if n := len(list("office", "")); n != 0 {
		t.Errorf("one with no role on the AP sees %d", n)
	}
	if len(list("griff", "?client="+mac)) != 1 || len(list("griff", "?client=02:00:00:00:00:01")) != 0 || len(list("griff", "?ap="+ap)) != 1 || len(list("griff", "?ap=office-ap")) != 0 {
		t.Errorf("the captures, by client and by AP")
	}

	// Read, it is one file: an interface for each, and their packets by time.
	code, head, file := f.file(fmt.Sprintf("/v1/pcaps/%d/file", id), "tenant")
	if code != 200 || head.Get("Content-Type") != "application/x-pcapng" || !strings.Contains(head.Get("Content-Disposition"), `filename="aeolus-ShedAP-72bb669f7694-`) {
		t.Fatalf("reading it: %d %v", code, head)
	}
	if n, err := pcapFrames(file); err != nil || n != 4 {
		t.Fatalf("the file read: %d frames, %v", n, err)
	}
	le := binary.LittleEndian
	var names, order []string
	for at := 0; at < len(file); at += int(le.Uint32(file[at+4:])) {
		body := file[at+8 : at+int(le.Uint32(file[at+4:]))-4]
		switch le.Uint32(file[at:]) {
		case pcapInterface:
			size := int(le.Uint16(body[10:]))
			names = append(names, fmt.Sprintf("link %d %s", le.Uint16(body), body[12:12+size]))
		case pcapFrame:
			order = append(order, fmt.Sprintf("%d@%d:%d/%d", le.Uint32(body), le.Uint32(body[8:]), le.Uint32(body[12:]), le.Uint32(body[16:])))
		}
	}
	if got, want := strings.Join(names, "; "), "link 1 phy0-ap1; link 1 eth0"; got != want {
		t.Errorf("its interfaces = %q, want %q", got, want)
	}
	us := func(sec, usec int) uint32 { return uint32((uint64(sec)*1e6 + uint64(usec)) & 0xffffffff) }
	want := fmt.Sprintf("0@%d:60/960 1@%d:64/964 0@%d:40/940 1@%d:44/944", us(100, 100), us(100, 200), us(100, 300), us(100, 400))
	if got := strings.Join(order, " "); got != want {
		t.Errorf("its packets = %q, want %q", got, want)
	}
	if code, _, _ := f.file(fmt.Sprintf("/v1/pcaps/%d/file", id), "office"); code != 404 {
		t.Errorf("one with no role on the AP reading it: %d", code)
	}

	// Any action asked of the AP (0104) is said on its line too: that
	// something waits, which it fetches at once.
	if code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "locate"}); code != 200 {
		t.Fatalf("asking the AP to blink: %d %v", code, body)
	}
	if said, _ := next(t, lines, "that something waits, on the AP's line", func(v map[string]any) bool { return v["poll"] != nil }); said["poll"] != true {
		t.Errorf("what the AP is told of an action: %v", said)
	}
	// An AP that holds no line is told nothing, and its capture says so.
	hang()
	for i := 0; i < 100 && len(f.api.live.on()) > 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	// A capture of a VLAN, the wired side alone, of whole packets.
	code, body = start("griff", map[string]any{"sides": "wired", "vlan": 20, "whole": true, "seconds": 30})
	if code != 200 {
		t.Fatalf("a capture of a VLAN: %d %v", code, body)
	}
	if body["capture"].(map[string]any)["told"] != nil {
		t.Errorf("a capture on an AP with no line says its AP was told: %v", body["capture"])
	}
	ask = body["capture"].(map[string]any)["ask"].(map[string]any)
	if ask["vlan"] != float64(20) || ask["snap"] != float64(0) || ask["bytes"] != float64(50<<20) || ask["network"] != nil {
		t.Errorf("what it asks: %v", ask)
	}
	second := int64(body["capture"].(map[string]any)["id"].(float64))
	if code, _ := f.do("GET", fmt.Sprintf("/v1/pcaps/%d/file", second), "griff", nil); code != 404 {
		t.Errorf("reading one nothing has come of: %d", code)
	}

	// Deleted, it is gone, its files with it.
	if code, body := f.do("DELETE", fmt.Sprintf("/v1/pcaps/%d", id), "tenant", nil); code != 200 {
		t.Fatalf("deleting it: %d %v", code, body)
	}
	if code, _, _ := f.file(fmt.Sprintf("/v1/pcaps/%d/file", id), "griff"); code != 404 {
		t.Errorf("reading it once deleted: %d", code)
	}
	if n := len(list("griff", "")); n != 1 {
		t.Errorf("captures left: %d, want the VLAN's alone", n)
	}
}

// A file cut short, as one still being written, gives its whole packets;
// one that is no pcap is left out.
func TestMergePcapsCutShort(t *testing.T) {
	a := pcapClassic(1, [3]int{5, 0, 20}, [3]int{6, 0, 20})
	var out bytes.Buffer
	srcs := []pcapSource{{r: bytes.NewReader(a[:len(a)-7]), name: "a"}, {r: strings.NewReader("not a capture at all, whatever it is"), name: "b"}, {r: bytes.NewReader(nil), name: "c"}}
	if err := mergePcaps(&out, srcs); err != nil {
		t.Fatal(err)
	}
	if n, err := pcapFrames(out.Bytes()); err != nil || n != 1 {
		t.Errorf("of a file cut short: %d frames, %v", n, err)
	}
}
