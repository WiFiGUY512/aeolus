package api

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A client is on one AP at a time (0185). An AP lists a client until
// hostapd times it out, minutes after it roamed away without saying so, so
// two APs' reports can list the same client at once (Griff, 2026-10-20:
// "this cannot be true at the same time"). activity keeps, for each
// client, the AP it was last active on and when, from each report as it
// comes: the report's time less how long the client had been idle. A
// client an AP lists that was active on another since is that AP's no
// longer, in the counts the manager keeps and on the page.
//
// Each AP's minutes (0207) say it within a minute (0251): a client that sent
// an AP anything in a minute was active on it then. The minute that put a
// client on an AP is kept with it, for a page that has no report of it
// there yet.
//
// Each AP says too, the moment it happens, that a client joined it or left
// it (0252). A join or a leave is exact, where a minute's activity may have
// been at any time in it: a minute begun before a client joined another AP,
// or left this one, does not put it back.
type activity struct {
	mu   sync.Mutex
	last map[string]activeOn
}

type activeOn struct {
	ap hierarchy.NodeID
	at time.Time
	// minute is the AP's minute that put it there (0251), where one did,
	// or what its join said (0252).
	minute *conditions.ClientMinute
	// exact is a join or a leave the AP said as it happened (0252); left a
	// leave: the client is on no AP since.
	exact, left bool
}

// from is the earliest the activity a record says was: for one a minute
// put there, that minute's start, as it may have been at any time in it.
func (o activeOn) from() time.Time {
	if o.minute != nil && !o.exact {
		return o.minute.At
	}
	return o.at
}

// activeForget is how long a client's last activity is kept: well past the
// five minutes hostapd keeps a client that went quiet.
const activeForget = time.Hour

// note takes in the clients an AP's report made at at lists, and says which
// of them were active on another AP since.
func (a *activity) note(ap hierarchy.NodeID, at time.Time, clients []wifiClient) map[string]bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		a.last = map[string]activeOn{}
	}
	for mac, o := range a.last {
		if at.Sub(o.at) > activeForget {
			delete(a.last, mac)
		}
	}
	var gone map[string]bool
	for _, c := range clients {
		here := at.Add(-time.Duration(c.InactiveMS) * time.Millisecond)
		o, ok := a.last[c.MAC]
		// On another AP since, or gone from this one (0252).
		if ok && (o.ap != ap || o.left) && !here.After(o.at) {
			if gone == nil {
				gone = map[string]bool{}
			}
			gone[c.MAC] = true
			continue
		}
		// Its minutes here since say it later (0251).
		if ok && o.ap == ap && o.at.After(here) {
			continue
		}
		a.last[c.MAC] = activeOn{ap: ap, at: here}
	}
	return gone
}

// minutes takes in the clients an AP's minutes say (0251). One that sent
// the AP anything in a minute was active on it then: at the minute's start
// and as many seconds as it was there. One hostapd keeps after it roamed
// away sends nothing, and says nothing of where it is.
func (a *activity) minutes(ap hierarchy.NodeID, list []conditions.ClientMinute) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		a.last = map[string]activeOn{}
	}
	for i := range list {
		c := list[i]
		if c.RxPackets <= 0 {
			continue
		}
		at := c.At.Add(time.Duration(c.Secs) * time.Second)
		o, ok := a.last[c.MAC]
		if ok && !at.After(o.at) {
			continue
		}
		// What it sent in a minute begun before it joined another AP, or
		// left this one, may have been before (0252).
		if ok && o.exact && (o.ap != ap || o.left) && c.At.Before(o.at) {
			continue
		}
		a.last[c.MAC] = activeOn{ap: ap, at: at, minute: &c}
	}
}

// moved takes in the clients an AP said joined it or left it (0252), each
// the moment it did. A join puts the client on the AP from then; a leave
// takes it off the AP, onto none. Either is passed over where something
// later is known of the client: a minute's activity counts from that
// minute's start. A leave from an AP the client is no longer on says
// nothing.
func (a *activity) moved(ap hierarchy.NodeID, list []clientMove) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		a.last = map[string]activeOn{}
	}
	for i := range list {
		m := list[i]
		o, ok := a.last[m.MAC]
		if ok && m.At.Before(o.from()) {
			continue
		}
		if m.Left {
			if ok && o.ap != ap && !o.left {
				continue
			}
			a.last[m.MAC] = activeOn{ap: ap, at: m.At, exact: true, left: true}
			continue
		}
		// Known on this AP later already, as by its minute.
		if ok && o.ap == ap && !o.left && !m.At.After(o.at) {
			continue
		}
		c := m.ClientMinute
		a.last[m.MAC] = activeOn{ap: ap, at: m.At, minute: &c, exact: true}
	}
}

// arrived is the clients an AP's minutes put on it since its report, that
// the report does not list (0251): each as its latest minute there says it,
// until the AP's next report says the rest. In order by MAC; nil where
// there are none.
func (a *activity) arrived(ap hierarchy.NodeID, st *conditions.State) []conditions.ClientMinute {
	var since time.Time
	listed := map[string]bool{}
	if st != nil {
		since = st.At
		var r struct {
			Clients []wifiClient `json:"clients"`
		}
		if json.Unmarshal(st.Report, &r) == nil {
			for _, c := range r.Clients {
				listed[c.MAC] = true
			}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []conditions.ClientMinute
	for mac, o := range a.last {
		if o.ap == ap && o.minute != nil && o.at.After(since) && !listed[mac] {
			out = append(out, *o.minute)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

// elsewhere is which of the clients an AP's report lists were active on
// another AP since, or left it since (0252), by MAC, in order; nil where
// none was or it has none.
func (a *activity) elsewhere(ap hierarchy.NodeID, st *conditions.State) []string {
	if st == nil {
		return nil
	}
	var r struct {
		Clients []wifiClient `json:"clients"`
	}
	if json.Unmarshal(st.Report, &r) != nil || len(r.Clients) == 0 {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, c := range r.Clients {
		here := st.At.Add(-time.Duration(c.InactiveMS) * time.Millisecond)
		if o, ok := a.last[c.MAC]; ok && (o.ap != ap || o.left) && o.at.After(here) {
			out = append(out, c.MAC)
		}
	}
	sort.Strings(out)
	return out
}
