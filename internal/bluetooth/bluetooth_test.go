package bluetooth

import "testing"

func TestNames(t *testing.T) {
	c, u, a := Size()
	if c < 3000 || u < 500 || a < 40 {
		t.Fatalf("table: %d companies, %d UUIDs, %d categories", c, u, a)
	}
	for _, x := range []struct{ got, want string }{
		{Company(0x004c), "Apple, Inc."},
		{Company(0x0006), "Microsoft"},
		{Company(0x0969), "Woan Technology (Shenzhen) Co., Ltd."},
		{Company(0xffff), ""},
		{UUID("FD3D"), "Woan Technology (Shenzhen) Co., Ltd."},
		{UUID("180f"), "Battery"},
		{UUID("0000"), ""},
		{Appearance(0x0040), "Phone"},
		{Appearance(0x03c1), "Human Interface Device"},
	} {
		if x.got != x.want {
			t.Errorf("got %q, want %q", x.got, x.want)
		}
	}
}

func TestKind(t *testing.T) {
	n := func(v int) *int { return &v }
	for _, c := range []struct {
		d          Device
		kind, what string
	}{
		{Device{Company: n(0x004c), Mfr: "12190000", Tracker: "apple_find_my"}, "tracker", "Apple Find My"},
		{Device{UUID16: []string{"fd5a"}, Tracker: "samsung_smarttag"}, "tracker", "Samsung SmartTag"},
		{Device{Company: n(0x004c), Mfr: "0215aabb", Beacon: "ibeacon"}, "beacon", "iBeacon"},
		{Device{Company: n(0x004c), Mfr: "1005031c"}, "personal", "Apple iPhone, iPad, Mac or Watch"},
		{Device{Company: n(0x004c), Mfr: "0719"}, "audio", "AirPods or Beats"},
		{Device{Company: n(0x0006), Mfr: "0109202249596059"}, "personal", "Windows device"},
		{Device{Company: n(0x0006), Mfr: "030080"}, "input", "accessory waiting to pair (Swift Pair)"},
		{Device{UUID16: []string{"fcd2"}}, "sensor", "BTHome"},
		{Device{Company: n(0x0969), Mfr: "de64ec95", UUID16: []string{"fd3d"}}, "", "SwitchBot"},
		{Device{Appearance: n(0x03c1)}, "input", "Human Interface Device"},
		{Device{Appearance: n(0x0000), Company: n(0x0059)}, "", "Nordic Semiconductor ASA"},
		{Device{UUID16: []string{"ff00"}}, "", ""},
		{Device{UUID16: []string{"feed"}}, "", "Tile, Inc."},
		{Device{}, "", ""},
	} {
		kind, what := Kind(c.d)
		if kind != c.kind || what != c.what {
			t.Errorf("%+v: %q, %q; want %q, %q", c.d, kind, what, c.kind, c.what)
		}
	}
}
