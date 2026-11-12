package api

import (
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
)

// The longest name a person may give a client (0153).
const clientNameMost = 64

// How long ago a client may last have been seen and still have the name it
// gave of itself sent to the page, for the lists of those on now.
const clientHostsFor = 30 * 24 * time.Hour

// clientNames is what clients are called, by MAC, of the clients last seen
// on an AP the caller may view: GET /v1/client-names. names is the names
// people gave them (0153). hosts is the name each last gave of itself, of
// those seen in the last thirty days: an AP's report says a client's name
// only while the AP remembers the client's DHCP, which it does not after
// its agent starts again or the client comes to it from another AP, and the
// manager has kept it. The page asks once a draw, and calls each client by
// the first of: the name a person gave it, the name its AP's report says,
// the name it last gave.
func (s *Server) clientNames(w http.ResponseWriter, _ *http.Request, c call) error {
	aps := mayView(c, "")
	names, err := s.conds.ClientNames(aps)
	if err != nil {
		return err
	}
	hosts, err := s.conds.ClientHosts(aps, time.Now().Add(-clientHostsFor))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"names": names, "hosts": hosts})
	return nil
}

// clientRename gives a client a name (0153), or with an empty one takes its
// name away: PUT /v1/clients/{mac}/name with {"name": "Bambu printer"}. It
// is for a client that has been seen, and needs operator on the AP it was
// last seen on, as putting it off that AP does (0107). The name is a
// person's note of what the device is: nothing is told to an AP, and no
// config changes.
func (s *Server) clientRename(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !macRE.MatchString(mac) {
		return badRequest("a client is named by its MAC, such as aa:bb:cc:00:11:22")
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	name := strings.Join(strings.Fields(body.Name), " ")
	if utf8.RuneCountInString(name) > clientNameMost {
		return badRequest("name: at most %d characters", clientNameMost)
	}
	for _, ch := range name {
		if unicode.IsControl(ch) || ch == utf8.RuneError {
			return badRequest("name: letters, digits, spaces and marks, on one line")
		}
	}
	cl, ok, err := s.conds.ClientOf(mac)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !ok || !may(c, change.Locations, t, cl.AP, access.View) {
		return errNotFound
	}
	if !may(c, change.Locations, t, cl.AP, access.Clients) {
		return &apiError{http.StatusForbidden, "naming a client needs the clients right on the AP it was last seen on (0177)"}
	}
	if err := s.conds.NameClient(mac, name, string(c.actor)); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"mac": mac, "name": name})
	return nil
}
