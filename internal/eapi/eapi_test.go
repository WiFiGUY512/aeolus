package eapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var res []json.RawMessage
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestParse(t *testing.T) {
	sw, err := Parse("192.168.1.1", fixture(t, "homelab"))
	if err != nil {
		t.Fatal(err)
	}
	if sw.Hostname != "homelab" || sw.FQDN != "homelab.example.com" || sw.MAC != "28:e7:1d:d2:d0:13" || sw.Model != "CCS-720DP-24ZS-2F" {
		t.Fatalf("switch = %+v", sw)
	}
	if !sw.Names("homelab.example.com.") || !sw.Names("HOMELAB") || !sw.Names("192.168.1.1") || sw.Names("tower-switch") || sw.Names("") {
		t.Error("Names")
	}
	if n, ok := sw.On("Ethernet28"); !ok || n.Device != "tower-switch.example.com" || n.DevicePort != "Ethernet18" {
		t.Errorf("On(Ethernet28) = %+v", n)
	}
	// A port channel's neighbour is its members'.
	if n, ok := sw.On("Port-Channel2"); !ok || n.Device != "C360-AP" {
		t.Errorf("On(Port-Channel2) = %+v", n)
	}
	if sw.Channel("Ethernet14") != "Port-Channel2" || sw.Channel("Ethernet28") != "" {
		t.Error("Channel")
	}
	if l, ok := sw.Where("aa:bb:cc:00:11:22", 20); !ok || l.Port != "Port-Channel2" {
		t.Errorf("Where = %+v", l)
	}
	if _, ok := sw.Where("aa:bb:cc:00:11:22", 10); ok {
		t.Error("learned on the wrong VLAN")
	}
	// The management port, with no address, is no interface of its.
	if len(sw.Interfaces) != 5 || sw.Interfaces[0].Name != "Vlan1" {
		t.Errorf("interfaces = %+v", sw.Interfaces)
	}
	if g := sw.Gateway("192.168.20.151"); g == nil || g.Name != "Vlan20" || g.Prefix.String() != "192.168.20.1/24" {
		t.Errorf("Gateway = %+v", g)
	}
	if sw.Gateway("172.16.0.1") != nil || sw.Gateway("not an address") != nil {
		t.Error("a gateway for an address it has no network of")
	}
	if !sw.Owns("10.0.1.249") || sw.Owns("10.0.1.250") {
		t.Error("Owns")
	}
	if len(sw.Default) != 1 || sw.Default[0] != (NextHop{Address: "10.0.1.250", Interface: "Vlan1010"}) {
		t.Errorf("default = %+v", sw.Default)
	}
	if a, ok := sw.ARPOf("10.0.1.250"); !ok || a.MAC != "2c:dd:e9:12:72:e2" || a.Interface != "Vlan1010" || a.Port != "Ethernet28" {
		t.Errorf("ARP = %+v", a)
	}
}

func TestParseNotLearned(t *testing.T) {
	sw, _ := Parse("x", fixture(t, "homelab"))
	if a, ok := sw.ARPOf("192.168.10.9"); !ok || a.Port != "" || a.Interface != "Vlan10" {
		t.Errorf("ARP = %+v", a)
	}
}

func TestParseRefusesWhatIsNotUnderstood(t *testing.T) {
	res := fixture(t, "homelab")
	if _, err := Parse("x", res[:3]); err == nil {
		t.Error("too few answers")
	}
	res[3] = json.RawMessage(`"no table"`)
	if _, err := Parse("x", res); err == nil || !strings.Contains(err.Error(), "show mac address-table") {
		t.Errorf("err = %v", err)
	}
}

func TestMAC(t *testing.T) {
	for in, want := range map[string]string{"2cdd.e912.72e2": "2c:dd:e9:12:72:e2", "28:E7:1D:D2:D0:13": "28:e7:1d:d2:d0:13", "00-0d-b9-aa-bb-cc": "00:0d:b9:aa:bb:cc"} {
		if got, ok := MAC(in); !ok || got != want {
			t.Errorf("MAC(%q) = %q", in, got)
		}
	}
	for _, bad := range []string{"", "homelab", "2cdd.e9fd.31e", "zzdd.e9fd.31e2"} {
		if _, ok := MAC(bad); ok {
			t.Errorf("MAC(%q) ok", bad)
		}
	}
}

func TestRead(t *testing.T) {
	answers := fixture(t, "homelab")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/command-api" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "aeolus" || p != "right" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Method string `json:"method"`
			Params struct {
				Cmds   []string `json:"cmds"`
				Format string   `json:"format"`
			} `json:"params"`
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "runCmds" || req.Params.Format != "json" || strings.Join(req.Params.Cmds, "|") != strings.Join(Commands, "|") {
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": 1002, "message": "CLI command 1 of 8 'show nonsense' failed: invalid command"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": answers})
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	hc := NewHTTP(5 * time.Second)
	ctx := context.Background()

	sw, err := Read(ctx, hc, host, "aeolus", "right")
	if err != nil || sw.Hostname != "homelab" || sw.Address != host || sw.At.IsZero() {
		t.Fatalf("Read = %+v, %v", sw, err)
	}
	if _, err := Read(ctx, hc, host, "aeolus", "wrong"); err == nil || !strings.Contains(err.Error(), "refuses the user or password") || strings.Contains(err.Error(), "wrong") {
		t.Errorf("a wrong password: %v", err)
	}
	if _, err := Run(ctx, hc, host, "aeolus", "right", []string{"show nonsense"}); err == nil || !strings.Contains(err.Error(), "invalid command") {
		t.Errorf("a command refused: %v", err)
	}
	// Sent elsewhere: not followed.
	moved := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/command-api", http.StatusTemporaryRedirect)
	}))
	defer moved.Close()
	if _, err := Read(ctx, hc, strings.TrimPrefix(moved.URL, "https://"), "aeolus", "right"); err == nil || !strings.Contains(err.Error(), "sends eAPI elsewhere") {
		t.Errorf("a redirect: %v", err)
	}
	srv.Close()
	// A client of its own, with no connection kept from before to be reset.
	if _, err := Read(ctx, NewHTTP(5*time.Second), host, "aeolus", "right"); err == nil || !strings.Contains(err.Error(), "refuses the connection") {
		t.Errorf("no switch: %v", err)
	}
}

// homelab's and tower-switch's eAPI offer AES256-SHA alone, TLS 1.2 with
// RSA key exchange, which Go's own client no longer offers.
func TestReadAnOldSwitch(t *testing.T) {
	answers := fixture(t, "homelab")
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "aeolus", "result": answers})
	}))
	srv.TLS = &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_256_CBC_SHA}}
	srv.StartTLS()
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	if sw, err := Read(context.Background(), NewHTTP(5*time.Second), host, "aeolus", "right"); err != nil || sw.Hostname != "homelab" {
		t.Fatalf("Read = %v, %v", sw, err)
	}
	// One whose TLS cannot be agreed is said to be that.
	plain := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	if _, err := Read(context.Background(), plain, host, "aeolus", "right"); err == nil || !strings.Contains(err.Error(), "no TLS could be agreed") {
		t.Errorf("no TLS: %v", err)
	}
}
