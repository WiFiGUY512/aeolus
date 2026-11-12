package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/maps"
)

// A folder's map is drawn from OpenStreetMap's tiles, its addresses found
// by OpenStreetMap's Nominatim, and suggested as they are typed by Photon
// (0169), all asked by the manager.
var (
	mapTiles     = maps.NewTiles("https://tile.openstreetmap.org")
	mapFinder    = maps.NewFinder("https://nominatim.openstreetmap.org")
	mapSuggester = maps.NewSuggester("https://photon.komoot.io")
)

// mapTile is one of the map's tiles (0169): GET /v1/map/tiles/{z}/{x}/{y},
// a PNG, for anyone signed in. The manager fetches it, and keeps it a week;
// past the rate it fetches at, the answer is 503, to be asked again.
func (s *Server) mapTile(w http.ResponseWriter, r *http.Request, _ call) error {
	var n [3]int
	for i, k := range []string{"z", "x", "y"} {
		v, err := strconv.Atoi(strings.TrimSuffix(r.PathValue(k), ".png"))
		if err != nil {
			return errNotFound
		}
		n[i] = v
	}
	data, err := mapTiles.Get(r.Context(), n[0], n[1], n[2])
	if errors.Is(err, maps.ErrNoTile) {
		return errNotFound
	}
	if errors.Is(err, maps.ErrBusy) {
		w.Header().Set("Retry-After", "5")
		return &apiError{http.StatusServiceUnavailable, err.Error()}
	}
	if err != nil {
		return &apiError{http.StatusBadGateway, err.Error()}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
	return nil
}

// mapFind is where an address may be (0169): GET /v1/map/find?q=, places,
// each {name, latitude, longitude}, the likeliest first, five at most. The
// manager asks OpenStreetMap, at most once a second.
func (s *Server) mapFind(w http.ResponseWriter, r *http.Request, _ call) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 3 || len(q) > 200 {
		return badRequest("say an address, of 3 to 200 characters")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	found, err := mapFinder.Find(ctx, q)
	if err != nil {
		return &apiError{http.StatusBadGateway, err.Error()}
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": found})
	return nil
}

// mapSuggest is where an address being typed may be (0169): GET
// /v1/map/suggest?q=, with ?lat= and ?lon= where the person is looking,
// places, each {name, latitude, longitude}, five at most, from Photon,
// those near where they look first among alike. The manager asks Photon
// at most four times a second, and keeps each answer a day.
func (s *Server) mapSuggest(w http.ResponseWriter, r *http.Request, _ call) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 3 || len(q) > 200 {
		return badRequest("say an address, of 3 to 200 characters")
	}
	var near *[2]float64
	if la, lo := r.URL.Query().Get("lat"), r.URL.Query().Get("lon"); la != "" || lo != "" {
		lat, err1 := strconv.ParseFloat(la, 64)
		lon, err2 := strconv.ParseFloat(lo, 64)
		if err1 != nil || err2 != nil || !(lat >= -90 && lat <= 90) || !(lon >= -180 && lon <= 180) {
			return badRequest("lat and lon are a latitude, -90 to 90, and a longitude, -180 to 180")
		}
		near = &[2]float64{lat, lon}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	found, err := mapSuggester.Suggest(ctx, q, near)
	if err != nil {
		if r.Context().Err() != nil {
			return nil // the page that asked is gone
		}
		return &apiError{http.StatusBadGateway, err.Error()}
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": found})
	return nil
}

// places is where a Locations node and everything below it is (0169): GET
// /v1/places?under=, the Org unless set. places has the node and each
// folder and AP below it the caller may view, each {node, name, kind
// (org, folder or ap), parent, address?, latitude?, longitude?, own?}: own
// where its place is set on it, not taken from a folder above it. A
// latitude and longitude set in different places are no place.
func (s *Server) places(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	if under == "" {
		under = t.Root()
	}
	if _, ok := t.Node(under); !ok || !may(c, change.Locations, t, under, access.View) {
		return errNotFound
	}
	out := []map[string]any{}
	for _, id := range append([]hierarchy.NodeID{under}, t.Descendants(under)...) {
		n, ok := t.Node(id)
		if !ok || !may(c, change.Locations, t, id, access.View) {
			continue
		}
		kind := "folder"
		switch n.Kind {
		case hierarchy.KindOrg:
			kind = "org"
		case hierarchy.KindAP:
			kind = "ap"
		}
		one := map[string]any{"node": id, "name": n.Name, "kind": kind, "parent": n.Parent}
		lat, okLat := t.Resolve(id, hierarchy.Path("system.place.latitude"))
		lon, okLon := t.Resolve(id, hierarchy.Path("system.place.longitude"))
		// A place is its latitude and longitude set on one node: one of each
		// from different places is no place.
		if a, b := number(lat.Value), number(lon.Value); okLat && okLon && lat.From == lon.From && a != nil && b != nil {
			one["latitude"], one["longitude"], one["own"] = *a, *b, lat.From == id
		}
		if a, ok := t.Resolve(id, hierarchy.Path("system.place.address")); ok {
			one["address"] = a.Value
		}
		out = append(out, one)
	}
	writeJSON(w, http.StatusOK, map[string]any{"under": under, "places": out})
	return nil
}

// number is a setting's number, as JSON or Go holds one.
func number(v any) *float64 {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	case json.Number:
		n, err := x.Float64()
		if err != nil {
			return nil
		}
		f = n
	default:
		return nil
	}
	return &f
}
