package compose

import "time"

// TestNetworkPath is the Locations field that switches a test network on
// (0179).
const TestNetworkPath = "test_network"

// TestNetworkMax is the longest a test network is switched on for at a time
// (0179).
const TestNetworkMax = 4 * time.Hour

// clockSet is the earliest clock an AP turns a test network on by, as its
// renderer has it (render.uc CLOCK_SET): before it, its clock is not set.
var clockSet = time.Unix(1700000000, 0)

// TestOn says whether test network id is switched on at now by an AP's
// config (0179): its test_network names it, and its time is not up.
func TestOn(doc map[string]any, id string, now time.Time) bool {
	on, _ := doc["test_network"].(map[string]any)
	name, _ := on["network"].(string)
	until, ok := on["until"].(float64)
	return name == id && ok && !now.Before(clockSet) && now.Unix() < int64(until)
}

// Live is an AP's config as it renders at now (0179): a test network is in
// it only while it is switched on there and its time is not up. The AP's
// renderer leaves the others out by the same rule, by its own clock, so the
// render check is asked about what the AP renders. doc is left as it is.
func Live(doc map[string]any, now time.Time) map[string]any {
	nets, _ := doc["network"].(map[string]any)
	var out map[string]any
	for id, n := range nets {
		net, _ := n.(map[string]any)
		if test, _ := net["test"].(bool); !test || TestOn(doc, id, now) {
			continue
		}
		if out == nil {
			out = make(map[string]any, len(doc))
			for k, v := range doc {
				out[k] = v
			}
			kept := make(map[string]any, len(nets))
			for k, v := range nets {
				kept[k] = v
			}
			out["network"] = kept
		}
		delete(out["network"].(map[string]any), id)
	}
	if out == nil {
		return doc
	}
	return out
}
