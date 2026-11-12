package api

import (
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Each radio's noise floor is the middle of its latest minutes' readings,
// with the lowest and highest of them; one below thermal noise is said to
// be no measurement, and one above -85 dBm loud. A minute older than the
// time is not counted, and one who may view no AP sees none.
func TestNoiseFloor(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	now := time.Now().UTC().Truncate(time.Minute)
	var minutes []conditions.RadioMinute
	add := func(radio, band string, ago time.Duration, dbm int) {
		n := dbm
		minutes = append(minutes, conditions.RadioMinute{AP: hierarchy.NodeID(ap), At: now.Add(-ago), Radio: radio, Band: band, Noise: &n})
	}
	for i, dbm := range []int{-92, -91, -90, -91, -93} {
		add("radio0", "5g", time.Duration(i+1)*time.Minute, dbm)
	}
	// Loud an hour ago: not in the time.
	add("radio0", "5g", time.Hour, -50)
	for i := range 3 {
		add("radio1", "2g", time.Duration(i+1)*time.Minute, -107)
		add("radio2", "6g", time.Duration(i+1)*time.Minute, -80)
	}
	if err := f.api.conds.RecordMinutes(minutes, nil); err != nil {
		t.Fatal(err)
	}
	code, body := f.do("GET", "/v1/noise", "griff", nil)
	if code != 200 || body["minutes"] != 15.0 || body["thermal"] != -101.0 || body["loud_over"] != -85.0 {
		t.Fatalf("%d %v", code, body)
	}
	radios := body["aps"].(map[string]any)[ap].(map[string]any)
	five := radios["radio0"].(map[string]any)
	if five["band"] != "5g" || five["dbm"] != -91.0 || five["low"] != -93.0 || five["high"] != -90.0 || five["minutes"] != 5.0 || five["unmeasured"] != nil || five["loud"] != nil {
		t.Errorf("5 GHz = %v", five)
	}
	if two := radios["radio1"].(map[string]any); two["unmeasured"] != true || two["loud"] != nil {
		t.Errorf("2.4 GHz, below thermal noise = %v", two)
	}
	if six := radios["radio2"].(map[string]any); six["loud"] != true || six["unmeasured"] != nil {
		t.Errorf("6 GHz, loud = %v", six)
	}
	if _, none := f.do("GET", "/v1/noise", "office", nil); len(none["aps"].(map[string]any)) != 0 {
		t.Errorf("one who may view no AP: %v", none)
	}
}
