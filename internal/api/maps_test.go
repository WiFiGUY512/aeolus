package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/maps"
)

// A folder's map (0169): where each folder and AP is, set where it is or
// taken from a folder above it; the map's tiles and the places an address
// may be, asked of OpenStreetMap by the manager. No AP is sent its place.
func TestPlacesAndTheMap(t *testing.T) {
	osm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/":
			w.Write([]byte(`{"features":[{"geometry":{"coordinates":[-86.301,32.3777]},"properties":{"name":"Alabama State Capitol","city":"Montgomery"}}]}`))
		case r.URL.Path == "/search":
			w.Write([]byte(`[{"display_name":"1 Main St, Montgomery, Alabama","lat":"32.3792","lon":"-86.3077"}]`))
		case strings.HasSuffix(r.URL.Path, ".png"):
			w.Write([]byte("\x89PNG tile " + r.URL.Path))
		default:
			http.NotFound(w, r)
		}
	}))
	defer osm.Close()
	tiles, finder, suggester := mapTiles, mapFinder, mapSuggester
	mapTiles, mapFinder, mapSuggester = maps.NewTiles(osm.URL), maps.NewFinder(osm.URL), maps.NewSuggester(osm.URL)
	defer func() { mapTiles, mapFinder, mapSuggester = tiles, finder, suggester }()

	f := newFixture(t)
	ap, token, version := f.adopted()
	set := func(node string, values map[string]any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": node, "values": values}); code != 200 {
			t.Fatalf("set: %d %v", code, b)
		}
	}
	set("house", map[string]any{"system.place.address": "1 Main St, Montgomery, AL", "system.place.latitude": 32.3792, "system.place.longitude": -86.3077})
	set(ap, map[string]any{"system.place.latitude": 32.38, "system.place.longitude": -86.31})
	// A latitude of its own with its folder's longitude is no place.
	set("office", map[string]any{"system.place.latitude": 10})
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house", "values": map[string]any{"system.place.latitude": 91}}); code == 200 {
		t.Errorf("a latitude past the pole: %v", b)
	}

	_, got := f.do("GET", "/v1/places?under=house", "griff", nil)
	by := map[string]map[string]any{}
	for _, p := range got["places"].([]any) {
		by[p.(map[string]any)["node"].(string)] = p.(map[string]any)
	}
	if h := by["house"]; h["kind"] != "folder" || h["own"] != true || h["latitude"] != 32.3792 || h["address"] != "1 Main St, Montgomery, AL" {
		t.Errorf("house = %v", h)
	}
	if o := by["office"]; o["latitude"] != nil || o["parent"] != "house" {
		t.Errorf("office, its latitude and house's longitude = %v", o)
	}
	if o := by["office-ap"]; o["own"] != nil || o["latitude"] != nil {
		t.Errorf("office-ap, under a mixed place = %v", o)
	}
	if a := by[ap]; a["kind"] != "ap" || a["own"] != true || a["longitude"] != -86.31 {
		t.Errorf("the AP, set on it = %v", a)
	}
	if len(by) != 4 { // house, office, office-ap and the adopted AP
		t.Errorf("places = %v", got["places"])
	}
	_, all := f.do("GET", "/v1/places", "griff", nil)
	if all["under"] != "example" || all["places"].([]any)[0].(map[string]any)["kind"] != "org" || all["places"].([]any)[0].(map[string]any)["latitude"] != nil {
		t.Errorf("the Org's = %v", all)
	}
	if code, _ := f.do("GET", "/v1/places?under=house", "tenant", nil); code != 404 {
		t.Errorf("to one who may not view it: %d", code)
	}
	if code, _ := f.do("GET", "/v1/places?under=nowhere", "griff", nil); code != 404 {
		t.Errorf("no such node: %d", code)
	}

	// No AP is sent its place, and placing it gives it no new version.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if cfg["version"] != version {
		t.Errorf("version %v, was %v", cfg["version"], version)
	}
	if b, _ := json.Marshal(cfg); strings.Contains(string(b), "place") || strings.Contains(string(b), "Montgomery") {
		t.Errorf("the AP's config has its place: %s", b)
	}

	// Where an address may be.
	_, found := f.do("GET", "/v1/map/find?q=1+Main+St+Montgomery", "griff", nil)
	if p := found["places"].([]any); len(p) != 1 || p[0].(map[string]any)["latitude"] != 32.3792 {
		t.Errorf("find = %v", found)
	}
	if code, _ := f.do("GET", "/v1/map/find?q=x", "griff", nil); code != 400 {
		t.Errorf("too short: %d", code)
	}

	// What an address being typed may be, near where the person looks.
	_, sug := f.do("GET", "/v1/map/suggest?q=alabama+cap&lat=32.37&lon=-86.30", "tenant", nil)
	if p := sug["places"].([]any); len(p) != 1 || p[0].(map[string]any)["name"] != "Alabama State Capitol, Montgomery" {
		t.Errorf("suggest = %v", sug)
	}
	for _, bad := range []string{"q=al", "q=alabama&lat=91&lon=0", "q=alabama&lat=32", "q=alabama&lat=NaN&lon=0"} {
		if code, _ := f.do("GET", "/v1/map/suggest?"+bad, "griff", nil); code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}

	// A tile, to anyone signed in, and to no one else.
	tile := func(path, as string) (int, string) {
		req, _ := http.NewRequest("GET", f.url+path, nil)
		if as != "" {
			req.Header.Set("Authorization", "Bearer "+f.tokens[as])
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type") + " " + string(b)
	}
	if code, b := tile("/v1/map/tiles/3/2/1", "tenant"); code != 200 || b != "image/png \x89PNG tile /3/2/1.png" {
		t.Errorf("tile = %d %q", code, b)
	}
	if code, _ := tile("/v1/map/tiles/3/2/1", ""); code != 401 {
		t.Errorf("signed out: %d", code)
	}
	if code, _ := tile("/v1/map/tiles/3/9/1", "griff"); code != 404 {
		t.Errorf("off the map: %d", code)
	}
}
