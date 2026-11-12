package conditions

import (
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What the APs' Bluetooth radios heard is kept (0158): a sighting once each
// five minutes, the strongest signal; each device's first and last time and
// the AP that heard it last; what it is, a word said once kept; the
// rotating addresses as counts; and all of it let go with the clients'
// history.
func TestBluetoothHistory(t *testing.T) {
	s, clock := open(t)
	now := *clock
	a, b := hierarchy.NodeID("ap-a"), hierarchy.NodeID("ap-b")
	sensor := func(at time.Time, rssi int, name string) BLEHeard {
		return BLEHeard{MAC: "C1:00:00:00:00:01", At: at, First: at.Add(-time.Hour), RSSI: rssi, Name: name, Kind: "sensor", What: "BTHome", Type: "random static"}
	}
	t0 := now.Add(-2 * time.Hour)
	// A report each minute from ap-a for twenty minutes, saying the sensor
	// again each time it was heard in the ten minutes before.
	for m := 0; m < 20; m++ {
		at := t0.Add(time.Duration(m) * time.Minute)
		if err := s.RecordBluetooth(a, []BLEHeard{sensor(at, -70+m%3, "")}, []BLECount{{What: "Microsoft · Windows device", At: at, Count: 1 + m%4}}); err != nil {
			t.Fatal(err)
		}
	}
	// ap-b hears it later, and gives its name; then ap-a again, with none.
	if err := s.RecordBluetooth(b, []BLEHeard{sensor(t0.Add(30*time.Minute), -60, "Shed sensor")}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordBluetooth(a, []BLEHeard{sensor(t0.Add(25*time.Minute), -75, "")}, nil); err != nil {
		t.Fatal(err)
	}
	d, ok, err := s.BluetoothDeviceKnown("c1:00:00:00:00:01")
	if err != nil || !ok {
		t.Fatalf("known: %v %v", ok, err)
	}
	if d.Name != "Shed sensor" || d.Kind != "sensor" || d.LastAP != b || !d.LastAt.Equal(t0.Add(30*time.Minute)) || !d.FirstAt.Equal(t0.Add(-time.Hour)) {
		t.Errorf("device: %+v", d)
	}
	seen, err := s.BluetoothSeen("c1:00:00:00:00:01", []hierarchy.NodeID{a, b}, now.Add(-24*time.Hour), 1000)
	if err != nil {
		t.Fatal(err)
	}
	// ap-a: minutes 0-19 are four five-minute slots, and 25 a fifth; ap-b one.
	if len(seen) != 6 || seen[0].AP != a || seen[0].RSSI != -68 || seen[len(seen)-1].AP != b {
		t.Errorf("seen: %+v", seen)
	}
	if only, _ := s.BluetoothSeen("c1:00:00:00:00:01", []hierarchy.NodeID{b}, now.Add(-24*time.Hour), 1000); len(only) != 1 {
		t.Errorf("by ap-b alone: %+v", only)
	}
	heard, err := s.BluetoothHeard([]hierarchy.NodeID{a, b}, now.Add(-24*time.Hour), 100)
	if err != nil || len(heard) != 1 || len(heard[0].APs) != 2 {
		t.Fatalf("heard: %+v %v", heard, err)
	}
	for _, h := range heard[0].APs {
		if (h.AP == a && (h.Slots != 5 || h.Best != -68)) || (h.AP == b && (h.Slots != 1 || h.Best != -60)) {
			t.Errorf("by %s: %+v", h.AP, h)
		}
	}
	if none, _ := s.BluetoothHeard([]hierarchy.NodeID{"ap-c"}, now.Add(-24*time.Hour), 100); len(none) != 0 {
		t.Errorf("an AP that heard nothing: %+v", none)
	}
	rot, err := s.BluetoothRotating([]hierarchy.NodeID{a}, now.Add(-24*time.Hour))
	if err != nil || len(rot) != 4 || rot[0].Count != 4 || rot[0].What != "Microsoft · Windows device" {
		t.Errorf("rotating: %+v %v", rot, err)
	}
	// Let go with the clients' history.
	*clock = now.Add(48 * time.Hour)
	if _, err := s.TrimConnections(24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.BluetoothDeviceKnown("c1:00:00:00:00:01"); ok {
		t.Error("the device is kept past its time")
	}
	if seen, _ := s.BluetoothSeen("c1:00:00:00:00:01", []hierarchy.NodeID{a, b}, time.Time{}, 1000); len(seen) != 0 {
		t.Errorf("sightings kept past their time: %d", len(seen))
	}
}
