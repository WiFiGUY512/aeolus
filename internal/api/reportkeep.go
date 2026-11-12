package api

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A state report is taken as much as it can be (0290). One of its lists
// that holds an element the manager cannot read, or that does not pass its
// check, is taken without it, and the rest of the report kept: the element
// is left out and counted, where before the whole report was refused, as
// v0.232.0's clients were on every AP for an hour (0289), and nothing in
// Aeolus said so. A report the manager cannot take at all, one that is no
// JSON object or that fails a check of the whole, is still refused. What
// was left out, and what was refused, the manager keeps for each AP, so
// that an alert says it.

// reportList is one list of a state report, by its path from the report's
// top (a list inside an object of the report is named by both, as rrm and
// neighbours), and how one of its elements is read and checked.
type reportList struct {
	path []string
	keep func(json.RawMessage) error
}

// element reads one element of a list as T, as strictly as the report is
// read, with no field it does not know, and checks it where check is set.
func element[T any](check func(T) error) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var v T
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			return err
		}
		if check == nil {
			return nil
		}
		return check(v)
	}
}

// okay makes a check that says yes or no into one that says why not.
func okay[T any](ok func(T) bool, why string) func(T) error {
	return func(v T) error {
		if !ok(v) {
			return badRequest("%s", why)
		}
		return nil
	}
}

// reportLists are the lists of a state report whose elements are taken one
// by one: each with the check that state's check holds each element of it
// to. What check holds the list to as a whole (how many, no two alike)
// stays the whole report's.
var reportLists = []reportList{
	{[]string{"radios"}, element(radioState.check)},
	{[]string{"bssids"}, element(bssidCheck)},
	{[]string{"ports"}, element(portState.check)},
	{[]string{"vlan_probes"}, element(vlanProbe.check)},
	{[]string{"uplink_vlans"}, element(uplinkVLAN.check)},
	{[]string{"on_air"}, element(onAirState.check)},
	{[]string{"clients"}, element(wifiClient.check)},
	{[]string{"others"}, element(okay(otherNetwork.ok, "others: not one of the AP's own networks as the agent writes them"))},
	{[]string{"radius"}, element(radiusState.check)},
	{[]string{"ft"}, element(okay(ftBSS.ok, "ft: not a key holder as the agent writes them"))},
	{[]string{"fw_crashes"}, element(fwCrash.check)},
	{[]string{"mixed_libs"}, element(mixedLib.check)},
}

// reportTrim is what was left out of one list of a report: the list, by
// its path, as clients or rrm.neighbours; how many of its elements, of how
// many; whether what was there was no list at all, and left out whole; and
// why the first was.
type reportTrim struct {
	Field   string
	Dropped int
	Of      int
	Whole   bool
	Why     string
}

// whyMost is how much of why an element was left out is kept.
const whyMost = 240

func shortWhy(s string) string {
	if len(s) > whyMost {
		return strings.TrimSpace(s[:whyMost]) + "…"
	}
	return s
}

// trimReport takes a state report's body, and gives it back with each of
// reportLists' bad elements left out, and what was left out. An error is a
// body that is no JSON object: such a report is refused.
func trimReport(body []byte) ([]byte, []reportTrim, error) {
	var top map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&top); err != nil {
		return nil, nil, badRequest("request body: %v", err)
	}
	if top == nil {
		return nil, nil, badRequest("request body: not a JSON object")
	}
	var trims []reportTrim
	for _, l := range reportLists {
		if t := trimAt(top, l.path, l); t != nil {
			trims = append(trims, *t)
		}
	}
	if len(trims) == 0 {
		return body, nil, nil
	}
	out, err := json.Marshal(top)
	if err != nil {
		return nil, nil, err
	}
	return out, trims, nil
}

// trimAt leaves out of one list of obj, at path, its bad elements, and says
// what it left out; nil where nothing was, or the list is not there. An
// object on the way that is not one is left to the report's own reading,
// which refuses it.
func trimAt(obj map[string]json.RawMessage, path []string, l reportList) *reportTrim {
	raw, ok := obj[path[0]]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	if len(path) > 1 {
		var sub map[string]json.RawMessage
		if json.Unmarshal(raw, &sub) != nil || sub == nil {
			return nil
		}
		t := trimAt(sub, path[1:], l)
		if t != nil {
			if b, err := json.Marshal(sub); err == nil {
				obj[path[0]] = b
			}
		}
		return t
	}
	field := strings.Join(l.path, ".")
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		delete(obj, path[0])
		return &reportTrim{Field: field, Whole: true, Why: shortWhy(err.Error())}
	}
	kept := make([]json.RawMessage, 0, len(items))
	t := reportTrim{Field: field, Of: len(items)}
	for _, it := range items {
		if err := l.keep(it); err != nil {
			t.Dropped++
			if t.Why == "" {
				t.Why = shortWhy(err.Error())
			}
			continue
		}
		kept = append(kept, it)
	}
	if t.Dropped == 0 {
		return nil
	}
	if b, err := json.Marshal(kept); err == nil {
		obj[path[0]] = b
	}
	return &t
}

// readReport reads and checks a state report from its request: the body,
// at most maxReport; its lists without their bad elements (trimReport),
// and what was left out; then the report itself, as strictly as ever, with
// no field the manager does not know, and its check. An error is a report
// refused whole.
func readReport(w http.ResponseWriter, r *http.Request) (stateReport, []reportTrim, error) {
	var req stateReport
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReport))
	if err != nil {
		return req, nil, badRequest("request body: %v", err)
	}
	body, trims, err := trimReport(body)
	if err != nil {
		return req, nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, nil, badRequest("request body: %v", err)
	}
	if err := req.check(); err != nil {
		return req, nil, err
	}
	return req, trims, nil
}

// reportBook keeps, for each AP, its state reports of the last
// alerts.ReportTroubleFor (0290): when each came, and of those, which were
// refused whole and why, and which lists of them had elements left out. It
// is kept in memory: a manager that started since says nothing of before,
// and an AP's next report says it again.
type reportBook struct {
	mu  sync.Mutex
	aps map[hierarchy.NodeID]*reportsOf
}

type reportsOf struct {
	came    []time.Time // each report, taken or refused
	refused []time.Time
	why     string    // why the latest refused was
	since   time.Time // when this run of refusals began
	lists   map[string]*listTrims
}

type listTrims struct {
	at    []time.Time
	last  reportTrim
	since time.Time
}

// reportsKept is how many of an AP's reports are kept at most: one a second
// for the whole window, which no agent sends.
const reportsKept = 1024

// reportsSince keeps of times those since from, at most reportsKept.
func reportsSince(times []time.Time, from time.Time) []time.Time {
	i := 0
	for i < len(times) && times[i].Before(from) {
		i++
	}
	times = times[i:]
	if len(times) > reportsKept {
		times = times[len(times)-reportsKept:]
	}
	return times
}

// note keeps one report of an AP's at now: refused where why is set, and
// otherwise taken with what trims says was left out.
func (b *reportBook) note(ap hierarchy.NodeID, now time.Time, trims []reportTrim, why string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.aps == nil {
		b.aps = map[hierarchy.NodeID]*reportsOf{}
	}
	r := b.aps[ap]
	if r == nil {
		r = &reportsOf{lists: map[string]*listTrims{}}
		b.aps[ap] = r
	}
	from := now.Add(-alerts.ReportTroubleFor)
	r.came = append(reportsSince(r.came, from), now)
	r.refused = reportsSince(r.refused, from)
	if why != "" {
		if len(r.refused) == 0 {
			r.since = now
		}
		r.refused = append(r.refused, now)
		r.why = shortWhy(why)
	}
	for _, t := range trims {
		l := r.lists[t.Field]
		if l == nil {
			l = &listTrims{}
			r.lists[t.Field] = l
		}
		l.at = reportsSince(l.at, from)
		if len(l.at) == 0 {
			l.since = now
		}
		l.at = append(l.at, now)
		l.last = t
	}
}

// of is what the manager could not take of an AP's reports in the last
// alerts.ReportTroubleFor, as of now; nil where there was nothing.
func (b *reportBook) of(ap hierarchy.NodeID, now time.Time) *alerts.ReportTrouble {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.aps[ap]
	if r == nil {
		return nil
	}
	from := now.Add(-alerts.ReportTroubleFor)
	r.came, r.refused = reportsSince(r.came, from), reportsSince(r.refused, from)
	out := &alerts.ReportTrouble{Reports: len(r.came), Refused: len(r.refused)}
	if out.Refused > 0 {
		out.Why, out.Since = r.why, r.since
	}
	for _, field := range slices.Sorted(maps.Keys(r.lists)) {
		l := r.lists[field]
		if l.at = reportsSince(l.at, from); len(l.at) == 0 {
			delete(r.lists, field)
			continue
		}
		out.Trimmed = append(out.Trimmed, alerts.ListTrimmed{
			Field: field, Reports: len(l.at), Dropped: l.last.Dropped, Of: l.last.Of, Whole: l.last.Whole, Why: l.last.Why, Since: l.since,
		})
	}
	if out.Refused == 0 && len(out.Trimmed) == 0 {
		if len(r.came) == 0 {
			delete(b.aps, ap)
		}
		return nil
	}
	return out
}
