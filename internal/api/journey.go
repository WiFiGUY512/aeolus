package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/journey"
)

// rejoinsRead is how many of a client's latest attempts its journey reads
// for its coming back again and again (0190): a day of one a minute.
const rejoinsRead = 1500

// clientJourney is one Wi-Fi client's history (0103): GET
// /v1/clients/{mac}?hours=, 24 unless set, at most 720, from the state
// reports of the APs the caller may view.
func (s *Server) clientJourney(w http.ResponseWriter, r *http.Request, c call) error {
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
	t := c.state.Org.Locations
	samples, err := s.samplesOf(c, mac, time.Now().Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		return err
	}
	j := journey.Build(mac, samples)
	now := time.Now()
	from := now.Add(-time.Duration(hours) * time.Hour)
	// Its retries, over its busy minutes alone (0303), from what each of its
	// minutes came to (0207) on the APs the caller may view, from the start
	// of its first session.
	since := from
	if len(j.Sessions) > 0 && j.Sessions[0].From.Before(since) {
		since = j.Sessions[0].From
	}
	minutes, err := s.conds.ClientMinutes(mac, since.Add(-time.Minute), mayView(c, ""))
	if err != nil {
		return err
	}
	journey.Resends(&j, journeyMinutes(minutes))
	// Coming online at one AP again and again (0190), from the APs' records
	// of its attempts, which see what reports minutes apart cannot. Those
	// that did not come online say where it did not stay (0200).
	attempts, err := s.conds.Connections(mac, mayView(c, ""), rejoinsRead, time.Time{})
	if err != nil {
		return err
	}
	var ons []journey.Online
	for _, a := range attempts {
		if a.Started.Before(from) {
			continue
		}
		name := string(a.AP)
		if n, ok := t.Node(a.AP); ok {
			name = n.Name
		}
		ons = append(ons, journey.Online{AP: a.AP, Name: name, At: a.Started, Tried: a.Outcome != "online" && a.Outcome != "connected", Signal: a.Signal()})
	}
	journey.Rejoins(&j, ons, rejoinsFewest, now)
	// Dropping off and coming back again and again, on whichever APs
	// (0259): from its leavings and its next attempt after each.
	drops, err := s.dropsOf(mayView(c, ""), mac, from, t)
	if err != nil {
		return err
	}
	journey.Drops(&j, drops, dropsFewest)
	// An address set by hand, weighed by the subnet its network has there;
	// and the address it holds now, held by another client too (0325).
	subnets, err := s.subnetsOf(now.Add(-subnetsFor))
	if err != nil {
		return err
	}
	held, holders, err := s.holdersOf(c, mac, j, now)
	if err != nil {
		return err
	}
	journey.Addresses(&j, subnets, held, holders, now)
	// What a person called it (0153), said to whoever may view the AP it
	// was last seen on.
	if cl, ok, err := s.conds.ClientOf(mac); err != nil {
		return err
	} else if ok && cl.Name != "" && may(c, change.Locations, t, cl.AP, access.View) {
		if n, ok, err := s.conds.ClientNamed(mac); err != nil {
			return err
		} else if ok {
			j.Name, j.NamedBy, j.NamedAt = n.Name, n.By, &n.At
		}
	}
	// An AP's scan radio, making client tests (0121), is no device to look
	// into: its signal and its moves are the tests', so what would be worth
	// a look in a client's journey is left out of its, and it says whose.
	// Said to one who may view its AP, or who sees it somewhere: to anyone
	// else it would say which MACs are an AP's, of a client they see
	// nothing of.
	own := testClients(c.state)
	if ap, ok := own[strings.ToLower(mac)]; ok && (len(j.Sessions) > 0 || may(c, change.Locations, t, ap, access.View)) {
		j.Tester = testerOf(c, own, mac)
		j.Issues = j.Issues[:0]
	}
	writeJSON(w, http.StatusOK, j)
	return nil
}

// journeyMinutes are a client's minutes as its journey counts them (0303).
func journeyMinutes(ms []conditions.ClientMinute) []journey.Minute {
	out := make([]journey.Minute, 0, len(ms))
	for _, m := range ms {
		out = append(out, journey.Minute{AP: m.AP, Band: m.Band, At: m.At, TxPackets: m.TxPackets, TxRetries: m.TxRetries, TxFailed: m.TxFailed, Bytes: m.TxBytes + m.RxBytes})
	}
	return out
}
