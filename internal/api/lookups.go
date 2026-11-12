package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/dnslog"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/journey"
)

// What a client's lookups are read up to (0154).
const (
	lookupsRead    = 4000 // lookups read of the DNS server's log at one asking
	lookupsNames   = 500  // names said of them
	lookupsPlaces  = 12   // domains said of them, those asked for most
	lookupsWindows = 4    // the times a client held an address that are asked about, the latest
	lookupsJoins   = 200  // its attempts to come online whose first lookups are read
)

// The DNS server is asked with a client of its own, which gives up on a
// server that does not answer.
var dnsLogHTTP = &http.Client{Timeout: 12 * time.Second}

// asksFor is the step an AP notes of a client's lookup as it comes online
// (0118): "asks 192.168.20.1 for mqtt.example.com (A)".
var asksFor = regexp.MustCompile(`^asks \S+ for (\S+) \(([A-Za-z0-9]+)\)$`)

// heldAddress is a time a client held an address, as its APs' reports say.
type heldAddress struct {
	Address string    `json:"address"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
}

// samplesOf is what the APs the caller may view reported of a client since
// a time: one sample a report it was in.
func (s *Server) samplesOf(c call, mac string, since time.Time) ([]journey.Sample, error) {
	t := c.state.Org.Locations
	viewable := map[hierarchy.NodeID]bool{}
	var samples []journey.Sample
	err := s.conds.StatesWith(since, mac, func(ap hierarchy.NodeID, st conditions.State) error {
		ok, seen := viewable[ap]
		if !seen {
			ok = may(c, change.Locations, t, ap, access.View)
			viewable[ap] = ok
		}
		if !ok {
			return nil
		}
		var rep struct {
			Clients []journey.Client `json:"clients"`
		}
		if json.Unmarshal(st.Report, &rep) != nil {
			return nil
		}
		name := string(ap)
		if n, ok := t.Node(ap); ok {
			name = n.Name
		}
		for _, cl := range rep.Clients {
			if strings.EqualFold(cl.MAC, mac) {
				samples = append(samples, journey.Sample{AP: ap, APName: name, At: st.At, C: cl})
			}
		}
		return nil
	})
	return samples, err
}

// heldAddresses is the times a client held each address, from its sessions: one
// after another with the same address is one time. A report shows a client
// some minutes after it came and before it went, so each time is taken
// half a report's gap wider, within since and now.
func heldAddresses(sessions []journey.Session, since, now time.Time) []heldAddress {
	var out []heldAddress
	for _, s := range sessions {
		if s.Address == "" || strings.Contains(s.Address, ":") {
			continue // none known, or IPv6, which the log is not asked by
		}
		from, to := s.From.Add(-journey.Gap/2), s.To.Add(journey.Gap/2)
		if from.Before(since) {
			from = since
		}
		if to.After(now) {
			to = now
		}
		if n := len(out); n > 0 && out[n-1].Address == s.Address {
			out[n-1].To = to
			continue
		}
		out = append(out, heldAddress{Address: s.Address, From: from, To: to})
	}
	if len(out) > lookupsWindows {
		out = out[len(out)-lookupsWindows:]
	}
	return out
}

// most is the domains asked for most, of a client's.
func most(places []dnslog.Place) []dnslog.Place {
	if len(places) > lookupsPlaces {
		places = places[:lookupsPlaces]
	}
	return places
}

// clientLookups is what a client looked up (0154): GET
// /v1/clients/{mac}/lookups, over the last ?hours= (24 unless set, at most
// 720), for whoever may view the AP it was last seen on.
//
// lookups is from the DNS server's own log, where one is set for that AP
// (system.dns_log): the names the client's address asked for while the
// client held it, each {name, types, times, first, last, blocked?,
// failed?}, the name asked for last first; places is the domains those
// names are in, each {domain, times, names}, the one asked for most first,
// a dozen at the most. server says the server asked,
// the times and addresses it was asked about, how many lookups its log has
// of them and how many were read; or, in error, why it could not be asked.
// The manager keeps none of it.
//
// joins is the names the client's APs noted as it came online (0118), its
// first few each time, from the attempts kept, and join_places their
// domains: there with or without a server.
func (s *Server) clientLookups(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !macRE.MatchString(mac) {
		return badRequest("a client is named by its MAC, such as aa:bb:cc:00:11:22")
	}
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		n, err := strconv.Atoi(h)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours is a whole number from 1 to 720")
		}
		hours = n
	}
	cl, ok, err := s.conds.ClientOf(mac)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !ok || !may(c, change.Locations, t, cl.AP, access.View) {
		return errNotFound
	}
	now := time.Now()
	since := now.Add(-time.Duration(hours) * time.Hour)
	out := map[string]any{"mac": mac, "hours": hours}

	// As it came online: what its APs noted, each time, over the hours
	// asked; and when they last noted any, for a client that asked for
	// nothing as it came online in that time (an ESPHome device, say).
	conns, err := s.conds.Connections(mac, mayView(c, ""), lookupsJoins, time.Time{})
	if err != nil {
		return err
	}
	var noted []dnslog.Entry
	for _, cn := range conns {
		var rec struct {
			Events []struct {
				T     int64  `json:"t"`
				Stage string `json:"stage"`
				What  string `json:"what"`
			} `json:"events"`
		}
		if json.Unmarshal(cn.Record, &rec) != nil {
			continue
		}
		for _, e := range rec.Events {
			if m := asksFor.FindStringSubmatch(e.What); e.Stage == "dns" && m != nil {
				at := cn.Started.Add(time.Duration(e.T) * time.Millisecond)
				if last, ok := out["joins_last"].(time.Time); !ok || at.After(last) {
					out["joins_last"] = at
				}
				if !at.Before(since) {
					noted = append(noted, dnslog.Entry{At: at, Name: m[1], Type: m[2]})
				}
			}
		}
	}
	joins := dnslog.Summarize(noted)
	out["join_places"] = most(dnslog.Places(joins))
	if len(joins) > lookupsNames {
		joins = joins[:lookupsNames]
	}
	out["joins"] = joins

	// The DNS server's own log, where one is set for where the client is.
	base, hasURL := t.Resolve(cl.AP, hierarchy.Path("system.dns_log.url"))
	sealed, hasToken := t.Resolve(cl.AP, hierarchy.Path("system.dns_log.token"))
	if !hasURL && !hasToken {
		writeJSON(w, http.StatusOK, out)
		return nil
	}
	server := map[string]any{}
	out["server"] = server
	u, _ := base.Value.(string)
	server["url"] = u
	plain, err := s.reveal("system.dns_log.token", sealed.Value)
	token, _ := plain.(string)
	if !hasURL || !hasToken || err != nil || token == "" {
		server["error"] = "the DNS server's log needs both its address and a token (System, What clients look up)"
		writeJSON(w, http.StatusOK, out)
		return nil
	}
	// Both set on one node, so that no one below where the token is set can
	// send it elsewhere (0168).
	if !together(base, sealed) {
		server["error"] = "the DNS server's address and token are set in different places: set both on one folder (System, What clients look up)"
		writeJSON(w, http.StatusOK, out)
		return nil
	}
	samples, err := s.samplesOf(c, mac, since)
	if err != nil {
		return err
	}
	asked := heldAddresses(journey.Build(mac, samples).Sessions, since, now)
	server["asked"] = asked
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var all []dnslog.Entry
	queries := 0
	// The latest time first, so that what is read, where not all is, is the
	// latest.
	for i := len(asked) - 1; i >= 0 && len(all) < lookupsRead; i-- {
		entries, total, err := dnslog.Technitium(ctx, dnsLogHTTP, u, token, asked[i].Address, asked[i].From, asked[i].To, lookupsRead-len(all))
		if err != nil {
			server["error"] = err.Error()
			break
		}
		all = append(all, entries...)
		queries += total
	}
	server["queries"], server["read"] = queries, len(all)
	lookups := dnslog.Summarize(all)
	// The domains it asks for most, of all that was read: what a device
	// asks for says what it is.
	out["places"] = most(dnslog.Places(lookups))
	if len(lookups) > lookupsNames {
		lookups = lookups[:lookupsNames]
	}
	out["lookups"] = lookups
	writeJSON(w, http.StatusOK, out)
	return nil
}

// What every client asks for most (0154), for the lists of clients: read of
// a DNS server's log at most this often, and this much of it.
const (
	topEvery   = 5 * time.Minute // how long what was read is kept before the log is asked again
	topRetry   = time.Minute     // and where it could not be read
	topRead    = 20000           // lookups read of the log, the latest
	topDomains = 3               // domains said of each client
)

// topKept is what one server's log said every address asked for, and when
// it was read.
type topKept struct {
	at    time.Time
	by    map[string]dnslog.Asked
	total int
	read  int
	err   string
}

// The logs read, by server and time asked about. One is read at a time: a
// page that asks again while one is read waits for it.
var topLogs = struct {
	sync.Mutex
	kept map[string]*topKept
}{kept: map[string]*topKept{}}

// topOf is what a server's log says every address asked for over the last
// hours: read anew where what is kept is old.
func topOf(ctx context.Context, base, token string, hours int) *topKept {
	sum := sha256.Sum256([]byte(token))
	key := base + "|" + strconv.Itoa(hours) + "|" + hex.EncodeToString(sum[:6])
	topLogs.Lock()
	defer topLogs.Unlock()
	now := time.Now()
	if k := topLogs.kept[key]; k != nil && (now.Sub(k.at) < topRetry || (k.err == "" && now.Sub(k.at) < topEvery)) {
		return k
	}
	k := &topKept{at: now}
	entries, total, err := dnslog.Technitium(ctx, dnsLogHTTP, base, token, "", now.Add(-time.Duration(hours)*time.Hour), now, topRead)
	if err != nil {
		k.err = err.Error()
	} else {
		k.by, k.total, k.read = dnslog.ByClient(entries, topDomains), total, len(entries)
	}
	topLogs.kept[key] = k
	return k
}

// lookupsTop is what each client on now asks for most (0154): GET
// /v1/lookups/top?under=, a Locations node for the APs below it, over the
// last ?hours= (24 unless set, at most 168). For each AP the caller may
// view there, with a DNS server's log set (system.dns_log), the clients its
// latest report has, by MAC: asks, each {total, places: [{domain, times,
// names}]}, its lookups in that time by its address now, and the three
// domains asked for most. servers says each log read: {url, queries, read,
// at}, or error. A log is read at most every five minutes, the latest
// twenty thousand lookups of it, and nothing of it is kept beyond that.
func (s *Server) lookupsTop(w http.ResponseWriter, r *http.Request, c call) error {
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		n, err := strconv.Atoi(h)
		if err != nil || n < 1 || n > 168 {
			return badRequest("hours is a whole number from 1 to 168")
		}
		hours = n
	}
	t := c.state.Org.Locations
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	asks := map[string]dnslog.Asked{}
	servers := []map[string]any{}
	said := map[string]bool{}
	for _, ap := range mayView(c, hierarchy.NodeID(r.URL.Query().Get("under"))) {
		base, hasURL := t.Resolve(ap, hierarchy.Path("system.dns_log.url"))
		sealed, hasToken := t.Resolve(ap, hierarchy.Path("system.dns_log.token"))
		if !hasURL || !hasToken || !together(base, sealed) {
			continue
		}
		u, _ := base.Value.(string)
		plain, err := s.reveal("system.dns_log.token", sealed.Value)
		token, _ := plain.(string)
		if err != nil || u == "" || token == "" {
			continue
		}
		l, err := s.conds.Latest(ap)
		if err != nil {
			return err
		}
		if l.State == nil {
			continue
		}
		var rep struct {
			Clients []struct {
				MAC     string `json:"mac"`
				Address string `json:"address"`
			} `json:"clients"`
		}
		if json.Unmarshal(l.State.Report, &rep) != nil || len(rep.Clients) == 0 {
			continue
		}
		k := topOf(ctx, u, token, hours)
		if !said[u] {
			said[u] = true
			one := map[string]any{"url": u, "at": k.at}
			if k.err != "" {
				one["error"] = k.err
			} else {
				one["queries"], one["read"] = k.total, k.read
			}
			servers = append(servers, one)
		}
		for _, cl := range rep.Clients {
			if a, ok := k.by[cl.Address]; ok && cl.Address != "" {
				asks[strings.ToLower(cl.MAC)] = a
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "asks": asks, "servers": servers})
	return nil
}
