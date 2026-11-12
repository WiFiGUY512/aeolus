package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/eapi"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/journey"
	"github.com/WiFiGUY512/aeolus/internal/oui"
	"github.com/WiFiGUY512/aeolus/internal/pathtrace"
)

// How the switches are read for a client's way out (0168).
const (
	switchesEvery = time.Minute      // how long what a switch said is kept before it is asked again
	switchesRetry = 15 * time.Second // and where it could not be read
	switchesWait  = 10 * time.Second // how long a switch is waited for
)

// The switches are asked with a client of their own, which takes their own
// certificates.
var switchHTTP = eapi.NewHTTP(switchesWait)

// switchKept is what one switch said, or why it could not be read, and
// when; done is closed once it is read, before which nothing of it is
// looked at.
type switchKept struct {
	at   time.Time
	sw   *eapi.Switch
	err  string
	done chan struct{}
}

// The switches read, or being read, by address, user and password.
var switchReads = struct {
	sync.Mutex
	kept map[string]*switchKept
}{kept: map[string]*switchKept{}}

// fresh says whether what a switch said may still be used.
func (k *switchKept) fresh(now time.Time) bool {
	return now.Sub(k.at) < switchesRetry || k.err == "" && now.Sub(k.at) < switchesEvery
}

// readSwitches is what each switch at addrs says: read anew, all at once,
// where what is kept is old; a read already under way is waited for, not
// made again. What is old is let go: nothing a switch said is kept past
// the minute it may be used.
func readSwitches(ctx context.Context, addrs []string, user, password string) []switchKept {
	sum := sha256.Sum256([]byte(password))
	key := func(a string) string { return a + "|" + user + "|" + hex.EncodeToString(sum[:6]) }
	now := time.Now()
	ks := make([]*switchKept, len(addrs))
	switchReads.Lock()
	for k, v := range switchReads.kept {
		select {
		case <-v.done:
			if !v.fresh(now) {
				delete(switchReads.kept, k)
			}
		default:
		}
	}
	for i, a := range addrs {
		if k := switchReads.kept[key(a)]; k != nil {
			ks[i] = k
			continue
		}
		k := &switchKept{at: now, done: make(chan struct{})}
		switchReads.kept[key(a)], ks[i] = k, k
		go func(k *switchKept, a string) {
			defer close(k.done)
			rctx, cancel := context.WithTimeout(context.Background(), switchesWait)
			defer cancel()
			sw, err := eapi.Read(rctx, switchHTTP, a, user, password)
			k.at = time.Now()
			if err != nil {
				k.err = err.Error()
			} else {
				k.sw = sw
			}
		}(k, a)
	}
	switchReads.Unlock()
	out := make([]switchKept, len(addrs))
	for i, k := range ks {
		select {
		case <-k.done:
			out[i] = switchKept{at: k.at, sw: k.sw, err: k.err}
		case <-ctx.Done():
			out[i] = switchKept{at: time.Now(), err: addrs[i] + " did not answer in time"}
		}
	}
	return out
}

// clientPath is a client's way out (0168): GET /v1/clients/{mac}/path, for
// whoever may view the AP it was last seen on. hops is its way, a hop at a
// time, from the client by its AP to the last thing the switches name before
// the internet, each {kind, name, address?, mac?, vendor?, model?, in?, out?,
// vlan?, gateway?, seen?, ssid?, band?, signal?, by, note?}; ends, why the way stops short, where it does.
// The switches are those set for that AP (system.switches), each read by its
// eAPI, at most once a minute: switches says each {address, name?, at} or
// error. on says whether the AP's latest report has the client still on it.
// internet is the AP's own check of the internet (0142), where it has one.
// The manager keeps none of it.
func (s *Server) clientPath(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !macRE.MatchString(mac) {
		return badRequest("a client is named by its MAC, such as aa:bb:cc:00:11:22")
	}
	cl, ok, err := s.conds.ClientOf(mac)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !ok || !may(c, change.Locations, t, cl.AP, access.View) {
		return errNotFound
	}
	st, err := s.conds.LatestState(cl.AP)
	if err != nil {
		return err
	}
	var rep struct {
		Clients        []journey.Client `json:"clients"`
		UplinkNeighbor *uplinkNeighbor  `json:"uplink_neighbor"`
		UplinkPort     *uplinkPort      `json:"uplink_port"`
		Path           *pathState       `json:"path"`
	}
	if st != nil {
		json.Unmarshal(st.Report, &rep)
	}

	client := pathtrace.Client{MAC: mac, Name: cl.Name, Address: cl.Address}
	if client.Name == "" {
		client.Name = cl.Host
	}
	on := false
	for _, x := range rep.Clients {
		if strings.EqualFold(x.MAC, mac) {
			on = true
			client.VLAN, client.Band, client.SSID, client.Signal = x.VLAN, x.Band, x.SSID, x.Signal
			if x.Address != "" {
				client.Address = x.Address
			}
		}
	}
	ap := pathtrace.AP{Name: string(cl.AP)}
	if n, ok := t.Node(cl.AP); ok {
		ap.Name = n.Name
	}
	if rep.Path != nil {
		ap.Address = rep.Path.Address
	}
	if rep.UplinkPort != nil {
		ap.Uplink = rep.UplinkPort.Name
	}
	if n := rep.UplinkNeighbor; n != nil {
		ap.Switch, ap.Port = n.System, n.Port
		if n.ChassisKind == "" || n.ChassisKind == "mac" {
			ap.Chassis = n.Chassis
		}
		for _, m := range n.Management {
			ap.Manage = append(ap.Manage, m.Address)
		}
	}
	out := map[string]any{"mac": mac, "ap": cl.AP, "on": on, "switches": []any{}}
	if rep.Path != nil && rep.Path.Internet != nil {
		out["internet"] = rep.Path.Internet
	}

	// The switches set for where the client is: their addresses, user and
	// password all set on one node, so that no one below where the password
	// is set can send it elsewhere (0168).
	var switches []*eapi.Switch
	ra, okA := t.Resolve(cl.AP, hierarchy.Path("system.switches.addresses"))
	ru, okU := t.Resolve(cl.AP, hierarchy.Path("system.switches.user"))
	rp, okP := t.Resolve(cl.AP, hierarchy.Path("system.switches.password"))
	addrs := stringsOf(ra.Value)
	user, _ := ru.Value.(string)
	var password string
	if okP {
		if plain, err := s.reveal("system.switches.password", rp.Value); err == nil {
			password, _ = plain.(string)
		}
	}
	switch {
	case len(addrs) > 0 && user != "" && password != "" && together(ra, ru, rp):
		// Each switch is read apart from the request, so that one closed
		// before the switches answer does not leave them said to be silent.
		ctx, cancel := context.WithTimeout(r.Context(), switchesWait+2*time.Second)
		defer cancel()
		var said []any
		for i, k := range readSwitches(ctx, addrs, user, password) {
			one := map[string]any{"address": addrs[i], "at": k.at}
			if k.err != "" {
				one["error"] = k.err
			} else {
				one["name"] = k.sw.Hostname
				switches = append(switches, k.sw)
			}
			said = append(said, one)
		}
		out["switches"] = said
	case okA && okU && okP:
		out["error"] = "the switches' addresses, user and password are set in different places: set all three on one folder (System, Switches the manager reads)"
	case okA || okU || okP:
		out["error"] = "the switches need their addresses, a user and its password (System, Switches the manager reads)"
	}
	tr := pathtrace.Walk(client, ap, switches)
	// Who made each thing with a MAC, where its MAC says: the device past
	// the last switch, named by nothing else, is known by it.
	// A VRRP or HSRP router's MAC is IANA's or Cisco's, not its maker's.
	for i := range tr.Hops {
		m := tr.Hops[i].MAC
		if strings.HasPrefix(m, "00:00:5e:") || strings.HasPrefix(m, "00:00:0c:07:ac:") {
			continue
		}
		if maker, private := oui.Lookup(m); maker != "" && !private {
			tr.Hops[i].Vendor = maker
		}
	}
	out["hops"] = tr.Hops
	if tr.Ends != "" {
		out["ends"] = tr.Ends
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// together says whether settings that name where the manager signs in,
// and with what, were set together (0168): on one node, and each there by
// a person, or each copied there by Break Hierarchy. A password set above,
// or copied down, never goes with an address set below.
func together(rs ...hierarchy.Resolved) bool {
	for _, r := range rs {
		if r.From != rs[0].From || (r.Origin == hierarchy.OriginBaseline) != (rs[0].Origin == hierarchy.OriginBaseline) {
			return false
		}
	}
	return true
}

// stringsOf is a list of strings, as a setting holds one.
func stringsOf(v any) []string {
	var out []string
	switch x := v.(type) {
	case []string:
		out = append(out, x...)
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}
