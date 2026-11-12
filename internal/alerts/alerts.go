// Package alerts says what needs a person's attention across the fleet
// (0099): an AP that stopped calling, a config the manager holds or an AP
// refused or put back, an agent update rolled back, and what an AP's own
// last report says is wrong, such as a tunnel down or a VLAN the switch
// does not send. It reads only what the manager has already; it stores
// nothing, so an alert ends when its cause does.
package alerts

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/journey"
	"github.com/WiFiGUY512/aeolus/internal/oui"
)

// The severities, most urgent first.
const (
	Critical = "critical" // clients are, or will be, without service
	Warning  = "warning"  // something is wrong that a person should look at
	Info     = "info"     // worth knowing, no harm yet
)

var rank = map[string]int{Critical: 0, Warning: 1, Info: 2}

// Alert is one thing that needs attention on one AP.
type Alert struct {
	AP       hierarchy.NodeID `json:"ap"`
	Name     string           `json:"name"`
	Severity string           `json:"severity"`
	Kind     string           `json:"kind"`
	Key      string           `json:"key"` // the kind, and what of the AP it is about: the same while the cause lasts
	// Short is the alert in a few words, as a list of them says it (0330):
	// what it is about and what is wrong, with the AP said beside it.
	// Message says it all.
	Short   string     `json:"short,omitempty"`
	Message string     `json:"message"`
	Since   *time.Time `json:"since,omitempty"`
	// Maintenance says the AP is in maintenance (0218), and why or until
	// when: its alert is listed and kept, and not sent or counted.
	Maintenance string `json:"maintenance,omitempty"`
}

// Input is what the manager knows of one AP.
type Input struct {
	AP         hierarchy.NodeID
	Name       string
	Now        time.Time
	Poll       time.Duration // between its config polls; 60 s unless set
	Unassigned bool          // waiting in Landing Zone
	Version    int64         // the config version it should run
	Since      time.Time     // when that version was made; zero if unknown
	WantsAgent string        // the agent bundle it should run (0079); empty if unknown
	Problems   []string      // why the manager holds its config, if it does
	Latest     conditions.Latest
	Fleet      *Fleet          // what is the fleet's own, to tell a stranger's network (0105)
	Known      map[string]bool // BSSIDs its folders' rogues.known say are no rogues (0106)
	Tests      []FailingTest   // its networks that fail their client tests here (0123)
	Joins      []FailingJoins  // its networks clients cannot get on (0127)
	Rejoins    []Rejoining     // its clients that come online again and again (0190)
	Struggles  []Struggling    // named clients that struggle on it (0233)
	PinsMoved  []PinMoved      // clients pinned to it heard far weaker than their usual here (0340)
	SlowRadios []SlowRadio     // its radios whose last client tests ran under a third of the usual (0242)
	OnRogue    []OnRogue       // clients of the APs' its scan radio heard on a rogue (0160)
	BLESilent  []BLESilent     // Bluetooth devices people named, this AP heard last, that no AP has heard for a day (0220)
	// DHCPServers is, by network, the DHCP servers its config says it has,
	// by address or MAC in lower case (0160): only another is an alert.
	DHCPServers map[string][]string
	// Again is where it enrolled again from, its token lost, where it waits
	// for a person to let it back in (0223).
	Again string
	// Zone is the time zone its config sets, system.tz, "" where none is
	// (0262). OnClock is what its config has the AP run by its own clock's
	// hours, and ByZone what the manager runs for it by Zone's: each as
	// "RRM's planned moves (02:00-05:00)".
	Zone            string
	OnClock, ByZone []string
	// Reports is what the manager could not take of its state reports in
	// the last ReportTroubleFor (0290); nil where it took them all.
	Reports *ReportTrouble
	// Minutes is when the newest minute the manager keeps of its radios
	// began (0311); nil where it was not looked up.
	Minutes *Minutes
	// Beacons is what each of its radios' beacons came to over the reports
	// of the last BeaconsWindow (0339); nil where none counts them, or they
	// were not looked up.
	Beacons []conditions.RadioBeacons
	// Uplink is what its uplink ports' minutes came to over the last
	// UplinkLossWindow (0351), the oldest first; nil where it was not looked
	// up, or none came.
	Uplink []conditions.UplinkMinute
}

// MinutesQuiet is how long an AP that calls, and has been up, may serve
// networks on a radio whose minutes do not come before that is an alert
// (0311): twice the 5 minutes after which its agent starts the sampler
// again (0307).
const MinutesQuiet = 10 * time.Minute

// Minutes is when the newest minute the manager keeps of an AP's radios
// began (0311): of each radio, over the last MinutesLooked; and of any
// radio, of all it keeps, zero where it keeps none.
type Minutes struct {
	Radios map[string]time.Time
	Last   time.Time
}

// MinutesLooked is how far back the newest minute of each radio is looked
// for (0311).
const MinutesLooked = 24 * time.Hour

// ReportTroubleFor is how long a state report the manager refused, or took
// with elements left out, is said in an alert (0290).
const ReportTroubleFor = 15 * time.Minute

// ReportTrouble is what the manager could not take of an AP's state reports
// in the last ReportTroubleFor (0290): how many it sent, how many of them
// were refused whole, why the latest was, and since when they have been;
// and each list of them that had elements left out.
type ReportTrouble struct {
	Reports, Refused int
	Why              string
	Since            time.Time
	Trimmed          []ListTrimmed
}

// ListTrimmed is one list of an AP's state reports, as clients or
// rrm.neighbours, that held elements the manager could not read, or that
// did not pass, and took without them (0290): in how many reports; in the
// latest, how many of how many, or that it was no list at all and left out
// whole; why; and since when.
type ListTrimmed struct {
	Field       string
	Reports     int
	Dropped, Of int
	Whole       bool
	Why         string
	Since       time.Time
}

// Rejoining is a client that came online at this AP again and again in the
// last hour (0190): it got on each time, and dropped its connection itself
// a minute or so in. Who it is, by the name it has and its MAC; how many
// times; about how long each time lasted; and since when. Same and Stays
// are the other APs it came online at in that hour, by name (0200): where
// it did the same, and where it stayed.
// OnRogue is a client the APs serve that this AP's scan radio heard sending
// to an AP that is not one of them, nor known (0160): as an evil twin of
// one of the networks would have it, or a network it was given by hand.
// Who is how it is called; Since, when it was first heard there.
type OnRogue struct {
	MAC, Who, BSSID, SSID string
	Since                 *time.Time
}

// DHCPKnown says whether a DHCP server, by its address and MAC, is one of
// those a network says it has (0160), by either, in any case.
func DHCPKnown(known []string, id, mac string) bool {
	for _, k := range known {
		if strings.EqualFold(k, id) || strings.EqualFold(k, mac) {
			return true
		}
	}
	return false
}

// dhcpHeard is a DHCP server an AP heard answer: the address it gave as its
// own, and its MAC.
type dhcpHeard struct {
	ID  string `json:"id"`
	MAC string `json:"mac"`
}

// DHCPNoAddress says whether a DHCP server's id, as an AP heard it, is no
// address of its own: what an answer gives that names none, as some
// servers' NAKs and broadcast answers do (2026-10-25: the tower switch's
// came as 0.0.0.0 and 255.255.255.255 beside its 192.168.1.254).
func DHCPNoAddress(id string) bool {
	return id == "" || id == "0.0.0.0" || id == "255.255.255.255"
}

// FoldDHCP folds each DHCP server heard with no address of its own into
// one of the same MAC, the one that gave an address where there is one:
// one server, not two. merge adds what the folded one says into the one it
// is folded into. Two of one MAC that each gave an address, as servers
// behind one relay, stay two.
func FoldDHCP[S any](servers []S, id, mac func(S) string, merge func(into *S, from S)) []S {
	out := make([]S, 0, len(servers))
	for _, s := range servers {
		if !DHCPNoAddress(id(s)) {
			out = append(out, s)
		}
	}
	for _, s := range servers {
		if !DHCPNoAddress(id(s)) {
			continue
		}
		same := func(o S) bool { return strings.EqualFold(mac(o), mac(s)) }
		i := slices.IndexFunc(out, func(o S) bool { return same(o) && !DHCPNoAddress(id(o)) })
		if i < 0 {
			i = slices.IndexFunc(out, same)
		}
		if i < 0 {
			out = append(out, s)
		} else if merge != nil {
			merge(&out[i], s)
		}
	}
	return out
}

// BLESilent is a Bluetooth device a person named that this AP heard last,
// and that no AP has heard since LastAt (0220).
type BLESilent struct {
	MAC, Name string
	LastAt    time.Time
}

// BLESilentAfter is how long a named Bluetooth device may go unheard before
// it is an alert: a sensor that says something every few minutes has gone
// quiet, its battery run down, or it was taken away. BLESilentFor is how
// long it stays one: past it, it is gone, and no news.
const (
	BLESilentAfter = 24 * time.Hour
	BLESilentFor   = 30 * 24 * time.Hour
)

// OnRogueFor is how recently a client must have been heard on a rogue for
// it to be an alert: it ends once the client is heard there no more.
const OnRogueFor = 15 * time.Minute

// Struggling is a client a person named that struggles on this AP (0233):
// who, by its name and MAC; how, a reason each; and since when.
type Struggling struct {
	MAC, Who string
	Why      []string
	Since    *time.Time
	// Better is where it did better lately (0309), where it did.
	Better *BetterPlace
}

// PinMoved is a client pinned to this AP (0319) that it has heard far
// weaker of late than it usually does (0340): moved, it may be pinned to an
// AP that is no longer its best. Its signal here over the last
// PinMovedRecent, at its middle, and its usual here, before that; since
// when it was weak; and where it did better lately, where it did.
type PinMoved struct {
	MAC, Who   string
	Band       string
	Now, Usual int
	Since      *time.Time
	Better     *BetterPlace
}

// A pinned client has moved (0340) where, over the last PinMovedRecent,
// PinMovedMinutes of its minutes or more heard it here at PinMovedWeak dBm
// or weaker at their middle, and PinMovedDB or more weaker than its usual
// here, the middle of its minutes over the PinMovedUsual before the last
// PinMovedGap, of PinMovedUsualMinutes at least: a client heard at -55 for
// a week and at -84 for half an hour was carried off.
const (
	PinMovedRecent       = 30 * time.Minute
	PinMovedMinutes      = 20
	PinMovedWeak         = -78
	PinMovedDB           = 10
	PinMovedUsual        = 7 * 24 * time.Hour
	PinMovedGap          = time.Hour
	PinMovedUsualMinutes = 60
)

// BetterPlace is an AP and band where a struggling client did well in the
// last two days (0309): its name, the band, the middle signal it was heard
// at there and the share of what was sent to it that was sent again, over
// how many busy minutes.
type BetterPlace struct {
	AP      string
	Band    string
	Signal  int
	Resent  float64
	Minutes int
}

// SlowRadio is a radio of this AP whose last client tests each ran under a
// third of what their testers usually get from it (0242): its band; the
// tests, the newest first; and since when, the oldest of them.
type SlowRadio struct {
	Band  string
	Tests []SlowTest
	Since *time.Time
}

// SlowTest is one of them: which AP tested; what it got each way, in
// Mbit/s; what that tester usually gets from this radio; and which way
// it was slow.
type SlowTest struct {
	Tester             string
	Down, Up           *float64
	UsualDown, UsualUp *float64
	SlowDown, SlowUp   bool
}

type Rejoining struct {
	MAC         string
	Who         string
	Times       int
	Every       time.Duration
	Since       *time.Time
	Same, Stays []string
	// Heard is how strongly this AP, and the others, heard it as it came
	// (0312).
	Heard journey.Signals
}

// FailingJoins is a network clients cannot get on at this AP (0127): of
// those let on to it here in the last hour, how many got no further; where
// most of them stopped and why, as the AP put it; and since when.
type FailingJoins struct {
	Network string
	SSID    string
	LetOn   int
	Failed  int
	Stage   string
	Reason  string
	Since   *time.Time
}

// FailingTest is a network that failed its last two client tests on this AP
// (0121, 0123): which, on which band, the AP whose scan radio tried it, by
// name, this AP itself where Itself; where the latest test stopped and
// why; and since when.
type FailingTest struct {
	Network string
	SSID    string
	Band    string
	Tester  string
	Itself  bool
	// How strongly the tester had the AP, in dBm, where its latest test
	// says: a failure from far off may be the distance's.
	Signal *float64
	Stage  string
	Reason string
	Since  *time.Time
}

// Fleet is what the APs together call their own (0105): every BSSID an AP
// reported as its own; every SSID of Aeolus's networks, as Heard makes
// them; and the middle four bytes of every address an AP calls its own
// (0160). A radio makes its BSSIDs from its address by changing the first
// and last bytes, and a C-360 hands them out anew when a radio starts
// again, so one an AP no longer names is still its own.
type Fleet struct {
	BSSIDs map[string]bool
	SSIDs  map[string]bool
	Addrs  map[string]bool
}

// Middle is the four bytes of a MAC between its first and last, as Fleet's
// Addrs keeps them, or "" for what is no MAC.
func Middle(mac string) string {
	if len(mac) != 17 {
		return ""
	}
	return strings.ToLower(mac[3:14])
}

// Own says whether a BSSID is one of the APs': one an AP names, or one made
// from an AP's address (0160).
func (f *Fleet) Own(bssid string) bool {
	if f.BSSIDs[bssid] {
		return true
	}
	m := Middle(bssid)
	return m != "" && f.Addrs[m]
}

// Heard is an SSID as an AP reports one it hears: its first 32 bytes,
// each not printable ASCII a ?, so Café is heard as Caf??.
func Heard(ssid string) string {
	b := []byte(ssid)
	if len(b) > 32 {
		b = b[:32]
	}
	for i, c := range b {
		if c < ' ' || c > '~' {
			b[i] = '?'
		}
	}
	return string(b)
}

// minUnanswered is how many of its clients' DHCP requests a network must
// leave unanswered in ten minutes, with none answered, to be silent.
const minUnanswered = 2

// RadioShortAfter is how long a radio must have had fewer of its networks
// on the air than it has before it is an alert (0216): past a config
// applied, a restart of the Wi-Fi, a driver loaded again; the AP does not
// count the minute a DFS channel listens for radar before it may send.
const RadioShortAfter = 4 * time.Minute

// A radio's beacons that did not go out (0221) are an alert at one in
// fifty, BeaconsFailingPct, over the reports of the last BeaconsWindow
// (0339), once they cover BeaconsSpan of it; and at least BeaconsFewest of
// them, so the few of a short time say nothing. Before they cover
// BeaconsSpan, as an AP starts, only one in ten, BeaconsStuckPct, is.
//
// A healthy radio misses well under one in a hundred: the five ath11k APs
// missed 0.19 to 0.35% on 2.4 GHz, 0.04 to 0.15% on 5 GHz and 0.02 to
// 0.03% on 6 GHz over a day (B169). They miss in bursts, of up to 5% over a
// minute, every hour or so on each AP, and the bursts of one AP are not
// another's: no harm to its clients, who leave only after a run of
// beacons missed. Judged a report at a time, one in fifty was an alert 63
// times in a week; over half an hour, the worst of two days was 1.65%.
const (
	BeaconsFailingPct = 2
	BeaconsStuckPct   = 10
	BeaconsFewest     = 30
	BeaconsWindow     = 30 * time.Minute
	BeaconsSpan       = 15 * time.Minute
)

// BeaconsJudged is the share of its beacons, in percent, at which a
// radio's over the last BeaconsWindow are an alert, and whether these are
// (0221, 0339): BeaconsFailingPct once they cover BeaconsSpan,
// BeaconsStuckPct before, and at least BeaconsFewest failed. The Radios
// page judges the half hours it shows by it too (0350).
func BeaconsJudged(b conditions.RadioBeacons) (pct int64, alert bool) {
	pct = BeaconsFailingPct
	if time.Duration(b.Over)*time.Second < BeaconsSpan {
		pct = BeaconsStuckPct
	}
	return pct, b.Failed >= BeaconsFewest && b.Failed*100 >= pct*(b.Sent+b.Failed)
}

// The wire as a choke point (0351). A minute of an uplink port is full where
// its busiest five seconds ran at UplinkFullPct of its link or more, either
// way, or its average did at UplinkAvgPct: UplinkFullMinutes of them in the
// last UplinkBusyWindow are an alert, as its clients were held back there by
// the wire and not the Wi-Fi. UplinkErrorsFewest errors in the last
// UplinkBusyWindow are one, and so is a link lost UplinkLossesFewest times
// in the last UplinkLossWindow. A healthy uplink in the lab had no errors and
// no link lost in days (2026-11-11).
const (
	UplinkFullPct      = 90
	UplinkAvgPct       = 70
	UplinkFullMinutes  = 3
	UplinkBusyWindow   = 15 * time.Minute
	UplinkErrorsFewest = 10
	UplinkLossesFewest = 2
	UplinkLossWindow   = time.Hour
)

// StateEvery is how often an AP reports its state (0040); a report older
// than three of these is too old to speak for the AP now.
const StateEvery = 5 * time.Minute

// Offline is how long an AP may be silent before it is called offline:
// three of its polls and half a minute, and never under five minutes, so a
// restart or a slow poll is not an alert.
func Offline(poll time.Duration) time.Duration {
	if poll <= 0 {
		poll = time.Minute
	}
	return max(3*poll+30*time.Second, 5*time.Minute)
}

// For is the alerts for one AP, most urgent first.
func For(in Input) []Alert {
	var out []Alert
	add := func(sev, kind, short, msg string, since *time.Time) {
		out = append(out, Alert{AP: in.AP, Name: in.Name, Severity: sev, Kind: kind, Key: kind, Short: short, Message: msg, Since: since})
	}
	l := in.Latest
	if in.Unassigned {
		add(Info, "unassigned", "waiting to be adopted", "waiting in Landing Zone to be adopted", seenAt(l.Seen))
		return out
	}
	if in.Again != "" {
		// It calls, as another box would that says it is this AP: until a
		// person lets it back in, it is as good as offline (0223).
		add(Critical, "again", "enrolled again: waits to be let in", fmt.Sprintf("enrolled again from %s, its token gone as after a reflash or a reset: it waits for a person to let it back in, on its page", in.Again), seenAt(l.Seen))
	} else if l.Seen == nil {
		add(Warning, "never-seen", "never called the manager", "has never called the manager", nil)
	} else if silent := in.Now.Sub(l.Seen.At); silent > Offline(in.Poll) {
		add(Critical, "offline", "offline for "+ago(silent), fmt.Sprintf("offline: last heard %s ago, from %s", ago(silent), l.Seen.Source), &l.Seen.At)
	}
	if len(in.Problems) > 0 {
		more := ""
		if len(in.Problems) > 1 {
			more = fmt.Sprintf(" (and %d more)", len(in.Problems)-1)
		}
		add(Warning, "held", "config held", fmt.Sprintf("config held: %s%s", in.Problems[0], more), nil)
	}
	failed := false
	if c := l.Check; c != nil && c.Version == in.Version && c.Result != conditions.OK && c.Result != conditions.Stale {
		failed = true
		why := c.Result
		if len(c.Problems) > 0 {
			why = c.Problems[0]
		}
		add(Critical, "refused", fmt.Sprintf("version %d refused", c.Version), fmt.Sprintf("version %d was refused by the render check: %s", c.Version, why), &c.At)
		out[len(out)-1].Key = fmt.Sprintf("refused:%d", c.Version) // each version's failure its own
	}
	if a := l.Apply; a != nil && a.Version == in.Version && !a.OK {
		failed = true
		add(Critical, "apply-failed", fmt.Sprintf("version %d did not apply", a.Version), fmt.Sprintf("version %d did not apply: %s", a.Version, a.Error), &a.At)
		out[len(out)-1].Key = fmt.Sprintf("apply-failed:%d", a.Version)
	}
	// Behind, with no reason above: it has not taken up the current version
	// for three polls since that version was made, though it calls in.
	if s := l.Seen; s != nil && !failed && len(in.Problems) == 0 && s.Running != nil && *s.Running != in.Version &&
		!in.Since.IsZero() && in.Now.Sub(in.Since) > Offline(in.Poll) && in.Now.Sub(s.At) <= Offline(in.Poll) {
		since := in.Since
		add(Warning, "behind", fmt.Sprintf("on version %d, not %d", *s.Running, in.Version), fmt.Sprintf("runs version %d, not the current %d", *s.Running, in.Version), &since)
	}
	if st := l.State; st != nil && in.Now.Sub(st.At) <= 3*StateEvery {
		out = append(out, fromReport(in, st)...)
	} else if s := l.Seen; st != nil && s != nil && in.Now.Sub(s.At) <= Offline(in.Poll) {
		// It calls and its reports stopped (0315): what is shown of it is
		// from its last, and nothing its reports would say is said. On
		// 2026-11-05 three APs' agents hung after a sysupgrade while their
		// live daemons went on calling, and nothing said so for half an
		// hour.
		since := st.At
		add(Warning, "reports-stopped", "no state report for "+ago(in.Now.Sub(st.At)), fmt.Sprintf("has sent no state report for %s, though it calls the manager: what Aeolus shows of it is from then, and the alerts its reports would raise are not raised",
			ago(in.Now.Sub(st.At))), &since)
	}
	// State reports the manager could not take, or took in part (0290):
	// what it shows of the AP is then older, or less, than the AP says.
	if t := in.Reports; t != nil {
		if t.Refused > 0 {
			since := t.Since
			add(Warning, "reports-refused", fmt.Sprintf("%d state reports refused", t.Refused), fmt.Sprintf("the manager refused %d of the %d state reports it sent in the last %s, and kept nothing of those: %s. What Aeolus shows of this AP may be out of date",
				t.Refused, t.Reports, ago(ReportTroubleFor), t.Why), &since)
		}
		for _, r := range t.Trimmed {
			what := fmt.Sprintf("%d of the %d in its latest", r.Dropped, r.Of)
			if r.Whole {
				what = "in its latest it was no list at all, and was left out whole"
			}
			since := r.Since
			add(Warning, "report-trimmed", "state reports trimmed: "+r.Field, fmt.Sprintf("its state reports' %s held what the manager could not take, which it left out, keeping the rest of each report: %s, in %d reports in the last %s: %s",
				r.Field, what, r.Reports, ago(ReportTroubleFor), r.Why), &since)
			out[len(out)-1].Key = "report-trimmed:" + r.Field
		}
	}
	// A network that failed its last two client tests here (0123): tried as
	// a client would, it does not work, whatever the AP says of itself.
	for _, t := range in.Tests {
		// Over the wire (0128): no radio had a part in it.
		if t.Band == "wire" {
			add(Warning, "client-test", t.SSID+" fails its wired test", fmt.Sprintf("%s does not work here over the wire, tried from this AP's uplink as a client would: it failed at %s: %s", t.SSID, t.Stage, t.Reason), t.Since)
			out[len(out)-1].Key = fmt.Sprintf("client-test:%s:%s:%s", t.Network, t.Band, t.Tester)
			continue
		}
		from := "from " + t.Tester
		if t.Itself {
			from = "from its own scan radio"
		} else if t.Signal != nil {
			from += fmt.Sprintf(", at %.0f dBm", *t.Signal)
		}
		on := ""
		if t.Band != "" {
			on = " on " + bandNames[t.Band]
		}
		add(Warning, "client-test", t.SSID+" fails its client test"+on, fmt.Sprintf("%s does not work here%s, tried as a client %s: it failed at %s: %s", t.SSID, on, from, t.Stage, t.Reason), t.Since)
		out[len(out)-1].Key = fmt.Sprintf("client-test:%s:%s:%s", t.Network, t.Band, t.Tester)
	}
	// A network clients cannot get on here (0127): by their own attempts,
	// as the AP recorded them (0118).
	for _, j := range in.Joins {
		where := ""
		if j.Stage != "" {
			where = fmt.Sprintf("; most stopped at %s: %s", stageNames[j.Stage], j.Reason)
		}
		add(Warning, "joins-failing", fmt.Sprintf("%d of %d cannot get on %s", j.Failed, j.LetOn, j.SSID), fmt.Sprintf("%d of the %d clients let on to %s here in the last hour got no further%s", j.Failed, j.LetOn, j.SSID, where), j.Since)
		id := j.Network
		if id == "" {
			id = "ssid:" + j.SSID
		}
		out[len(out)-1].Key = "joins-failing:" + id
	}
	// A client that comes online here again and again (0190): it got on,
	// and gave up on the connection itself, which the AP does not say. It
	// blames this AP's radio only where the client stayed on another AP in
	// that hour, and the client or what is beyond the APs where it did the
	// same on every other it joined (0200).
	// A Bluetooth device a person named that has gone quiet (0220).
	for _, b := range in.BLESilent {
		last := b.LastAt
		add(Info, "ble-silent", b.Name+" silent a day", fmt.Sprintf("Bluetooth device %s (%s), which this AP heard last, has not been heard by any AP for a day or more: a sensor whose battery ran down, or one taken away", b.Name, b.MAC), &last)
		out[len(out)-1].Key = "ble-silent:" + b.MAC
	}
	// A client of the APs' heard on a rogue (0160).
	for _, r := range in.OnRogue {
		named := r.BSSID
		if r.SSID != "" {
			named += fmt.Sprintf(" (%q)", r.SSID)
		}
		add(Warning, "client-on-rogue", whoName(r.Who, r.MAC)+" on a rogue AP", fmt.Sprintf("%s, a client of the APs', was heard sending to %s, which is not one of the APs': an evil twin of one of your networks, or a network it was given by hand. Mark the AP known (rogues.known) if it is yours", r.Who, named), r.Since)
		out[len(out)-1].Key = "client-on-rogue:" + r.MAC + "@" + r.BSSID
	}
	for _, r := range in.Struggles {
		msg := fmt.Sprintf("%s struggles: %s", r.Who, strings.Join(r.Why, "; "))
		if b := r.Better; b != nil {
			msg += fmt.Sprintf(". In the last two days it did better on %s (%s): heard at %d dBm, %.0f%% sent again over %d busy minutes. It may be stuck here: sticky steering on its network (0289) would move it",
				b.AP, bandName(b.Band), b.Signal, 100*b.Resent, b.Minutes)
		}
		add(Warning, "struggling", whoName(r.Who, r.MAC)+" struggles", msg, r.Since)
		out[len(out)-1].Key = "struggling:" + r.MAC
	}
	// A client pinned here that has moved (0340): its pin keeps it on an AP
	// that may no longer be its best (Griff, 2026-11-08).
	for _, r := range in.PinsMoved {
		msg := fmt.Sprintf("%s is pinned here, and over the last half hour it was heard on %s at %d dBm, %d dB weaker than its usual %d dBm here: it may have been moved, and be pinned to an AP that is no longer its best",
			r.Who, bandName(r.Band), r.Now, r.Usual-r.Now, r.Usual)
		if b := r.Better; b != nil {
			msg += fmt.Sprintf(". In the last day it did better on %s (%s), at %d dBm over %d minutes", b.AP, bandName(b.Band), b.Signal, b.Minutes)
		}
		add(Warning, "pin-moved", whoName(r.Who, r.MAC)+" pinned, moved?", msg+". Its pin is in its Steering", r.Since)
		out[len(out)-1].Key = "pin-moved:" + r.MAC
	}
	// A radio that runs slowly (0242): its last client tests, a client at
	// a place that does not move, each got under a third of the usual.
	for _, r := range in.SlowRadios {
		got := make([]string, len(r.Tests))
		var usual []string
		said := map[string]bool{}
		down, up := true, true
		for i, t := range r.Tests {
			got[i] = speedPair(t.Down, t.Up)
			down, up = down && t.SlowDown, up && t.SlowUp
			if !said[t.Tester] {
				said[t.Tester] = true
				usual = append(usual, speedPair(t.UsualDown, t.UsualUp)+" from "+t.Tester)
			}
		}
		what := "its clients get little from it"
		switch {
		case down && up:
			what = "it sends and receives slowly"
		case down:
			what = "it sends slowly"
		case up:
			what = "it receives slowly"
		}
		// One tester's tests alone may be its way to the radio, not the
		// radio (0308): the alert says so, and what would tell.
		if len(said) == 1 {
			who := r.Tests[0].Tester
			what += fmt.Sprintf(". All are %s's tests, so it may be %s's way to it and not the radio: a test from another AP would tell", who, who)
		}
		add(Warning, "slow-radio", "slow on "+bandName(r.Band), fmt.Sprintf("its last %d client tests on %s ran %s Mbit/s down/up, each under a third of what its testers usually get here (%s): %s",
			len(got), bandName(r.Band), andList(got), strings.Join(usual, ", "), what), r.Since)
		out[len(out)-1].Key = "slow-radio:" + r.Band
	}
	for _, r := range in.Rejoins {
		add(Warning, "rejoins", fmt.Sprintf("%s rejoins: %d times in an hour", whoName(r.Who, r.MAC), r.Times), fmt.Sprintf("%s came online here %d times in the last hour, about every %s", r.Who, r.Times, r.Every.Round(time.Second))+journey.Blame("this AP", r.Same, r.Stays, r.Heard), r.Since)
		out[len(out)-1].Key = "rejoins:" + r.MAC
	}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// stageNames are the stages of coming online (0118), as an alert says them.
var stageNames = map[string]string{"auth": "authentication", "assoc": "association", "signin": "the sign-in", "key": "the keys",
	"dhcp": "DHCP", "gateway": "the gateway", "dns": "DNS", "internet": "the first connection"}

var bandNames = map[string]string{"2g": "2.4 GHz", "5g": "5 GHz", "6g": "6 GHz"}

// report is the parts of a state report alerts read (0040, 0059, 0061,
// 0064, 0065, 0069, 0079).
type report struct {
	WirelessMissing bool `json:"wireless_missing"`
	// How long it has been up, in seconds, when it made the report.
	Uptime *int64 `json:"uptime"`
	// Each radio's networks on the air, of those it has, and for how long
	// fewer (0216).
	Radios []struct {
		Radio    string   `json:"radio"`
		Band     string   `json:"band"`
		BSS      *int     `json:"bss"`
		BSSWant  *int     `json:"bss_want"`
		ShortFor *int     `json:"short_for"`
		Missing  []string `json:"missing"`
	} `json:"radios"`
	// Its Ethernet ports (0053, 0093): their link, speed and duplex as
	// "1000F", and the fastest each end offers, in Mbit/s.
	Ports []struct {
		Name       string `json:"name"`
		Up         bool   `json:"up"`
		Speed      string `json:"speed"`
		Max        *int   `json:"max"`
		PartnerMax *int   `json:"partner_max"`
	} `json:"ports"`
	Agent *struct {
		Hash   string `json:"hash"`
		Update *struct {
			Version string `json:"version"`
			Hash    string `json:"hash"`
			State   string `json:"state"`
			Why     string `json:"why"`
			Ago     *int   `json:"ago"`
		} `json:"update"`
	} `json:"agent"`
	Transports map[string]struct {
		Active       string `json:"active"`
		CannotSwitch string `json:"cannot_switch"`
	} `json:"transports"`
	// Its addresses on networks its own Wi-Fi serves (0209).
	OnAir []struct {
		Interface  string   `json:"interface"`
		Management bool     `json:"management"`
		VLAN       *int     `json:"vlan"`
		SSIDs      []string `json:"ssids"`
	} `json:"on_air"`
	OffAir map[string]struct {
		Off bool   `json:"off"`
		Ago *int   `json:"ago"`
		Why string `json:"why"`
	} `json:"off_air"`
	// Its radios whose firmware crashed (0310).
	FWCrashes []struct {
		Ago       *int     `json:"ago"`
		Driver    string   `json:"driver"`
		Device    string   `json:"device"`
		Why       *string  `json:"why"`
		Recovered bool     `json:"recovered"`
		Lost      bool     `json:"lost"`
		Radios    []string `json:"radios"`
		Bands     []string `json:"bands"`
		Scan      bool     `json:"scan"`
	} `json:"fw_crashes"`
	// The libraries a process holds two builds of at once (0317).
	MixedLibs []struct {
		Lib    string   `json:"lib"`
		Builds []string `json:"builds"`
		Procs  []string `json:"procs"`
	} `json:"mixed_libs"`
	VXLAN *struct {
		Tunnels []struct {
			VNI     int    `json:"vni"`
			Peer    string `json:"peer"`
			Standby bool   `json:"standby"`
			Gone    bool   `json:"gone"`
			Probe   *struct {
				Verdict string `json:"verdict"`
			} `json:"probe"`
		} `json:"tunnels"`
		Loops []struct {
			Port string `json:"port"`
			VNI  int    `json:"vni"`
			Ago  *int   `json:"ago"`
		} `json:"loops"`
	} `json:"vxlan"`
	UplinkVLANs []struct {
		VLAN    int    `json:"vlan"`
		Verdict string `json:"verdict"`
	} `json:"uplink_vlans"`
	DHCP map[string]struct {
		Answered   int         `json:"answered"`
		Unanswered int         `json:"unanswered"`
		Servers    []dhcpHeard `json:"servers"`
	} `json:"dhcp"`
	Radius []struct {
		Network string `json:"network"`
		Server  string `json:"server"`
		Verdict string `json:"verdict"`
	} `json:"radius"`
	ScanRadio *struct {
		State      string `json:"state"`
		Networks   *int   `json:"networks"`
		LookedAgo  *int64 `json:"looked_ago"`
		Heals      *int   `json:"heals"`
		HealFailed *int   `json:"heal_failed"`
	} `json:"scan_radio"`
	Health *struct {
		CPU          *int     `json:"cpu"`
		MemTotal     int64    `json:"mem_total"`
		MemAvailable int64    `json:"mem_available"`
		StorageTotal int64    `json:"storage_total"`
		StorageFree  int64    `json:"storage_free"`
		Temp         *float64 `json:"temp"`
	} `json:"health"`
	Time *struct {
		Synced *bool   `json:"synced"`
		Rule   *string `json:"rule"` // the clock's zone, as a POSIX rule (0262)
	} `json:"time"`
	RRM *struct {
		// The channels it rated, by band, and whether each is its best
		// (0073): where its radio would move (0267).
		Ratings []struct {
			Band  string `json:"band"`
			Best  bool   `json:"best"`
			Stale bool   `json:"stale"`
		} `json:"ratings"`
		Others []struct {
			BSSID   string  `json:"bssid"`
			SSID    string  `json:"ssid"`
			Band    string  `json:"band"`
			Channel int     `json:"channel"`
			Signal  float64 `json:"signal"`
			Ago     *int    `json:"ago"`
		} `json:"others"`
		// Power control's radios, with what it says of their clients
		// (0206).
		APC []struct {
			Band        string `json:"band"`
			Power       int    `json:"power"`
			Clients     string `json:"clients"`
			ClientFloor *int   `json:"client_floor"`
			Lonely      []struct {
				MAC      string `json:"mac"`
				Est      int    `json:"est"`
				Measured bool   `json:"measured"`
			} `json:"lonely"`
			Distress *struct {
				At       int64      `json:"at"`
				Why      string     `json:"why"`
				Scanning int        `json:"scanning"`
				Left     int        `json:"left"`
				Ratio    []*float64 `json:"ratio"`
				Acted    bool       `json:"acted"`
			} `json:"distress"`
		} `json:"apc"`
		// The radios that can't scan (0120, 0263): band, channel, why
		// (dfs or refused), seconds since, and where a radio that looks
		// before it uses a DFS channel steps off to.
		CannotScan []struct {
			Band    string `json:"band"`
			Channel int    `json:"channel"`
			Why     string `json:"why"`
			Ago     int    `json:"ago"`
			Home    int    `json:"home"`
		} `json:"cannot_scan"`
	} `json:"rrm"`
	Airscan *struct {
		BSS []struct {
			BSSID   string  `json:"bssid"`
			SSID    string  `json:"ssid"`
			Band    string  `json:"band"`
			Channel int     `json:"channel"`
			Signal  float64 `json:"signal"`
			Wired   *bool   `json:"wired"`
			VLANs   []int   `json:"vlans"`
			Ago     *int    `json:"ago"`
		} `json:"bss"`
	} `json:"airscan"`
}

// heardNet is a network an AP hears, by its RRM's scans or its scan radio's
// (0105, 0160), and whether the scan radio found it on the wire, and on
// which VLANs.
type heardNet struct {
	BSSID, SSID, Band string
	Channel           int
	Signal            float64
	Ago               *int
	Wired             bool
	VLANs             []int
}

// heard is the networks the report says its AP hears, once each by BSSID:
// RRM's first, then those only the scan radio heard.
func (r report) heard() []heardNet {
	var out []heardNet
	at := map[string]int{}
	if r.RRM != nil {
		for _, o := range r.RRM.Others {
			at[o.BSSID] = len(out)
			out = append(out, heardNet{BSSID: o.BSSID, SSID: o.SSID, Band: o.Band, Channel: o.Channel, Signal: o.Signal, Ago: o.Ago})
		}
	}
	if r.Airscan != nil {
		for _, b := range r.Airscan.BSS {
			wired := b.Wired != nil && *b.Wired
			if i, ok := at[b.BSSID]; ok {
				if wired {
					out[i].Wired, out[i].VLANs = true, b.VLANs
				}
				continue
			}
			at[b.BSSID] = len(out)
			out = append(out, heardNet{BSSID: b.BSSID, SSID: b.SSID, Band: b.Band, Channel: b.Channel, Signal: b.Signal, Ago: b.Ago, Wired: wired, VLANs: b.VLANs})
		}
	}
	return out
}

func fromReport(in Input, st *conditions.State) []Alert {
	var r report
	if json.Unmarshal(st.Report, &r) != nil {
		return nil
	}
	var out []Alert
	// Since is when the report says the cause began, where it says, by how
	// long before it was made; else when the report was made.
	var ago *int
	add := func(sev, kind, subject, short, msg string) {
		key := kind
		if subject != "" {
			key += ":" + subject
		}
		since := st.At
		if ago != nil && *ago >= 0 {
			since = st.At.Add(-time.Duration(*ago) * time.Second)
		}
		ago = nil
		out = append(out, Alert{AP: in.AP, Name: in.Name, Severity: sev, Kind: kind, Key: key, Short: short, Message: msg, Since: &since})
	}
	if r.WirelessMissing {
		add(Critical, "wireless-missing", "", "netifd lost the radios", "netifd lost its network.wireless object: the radios run, but nothing sees them or their clients until the network restarts")
	}
	// A radio with networks it has not on the air, past a restart or an
	// apply (0216): none of them, and its clients have no service on that
	// band here; some, and theirs have none.
	for _, rd := range r.Radios {
		if rd.BSS == nil || rd.BSSWant == nil || rd.ShortFor == nil || *rd.BSSWant == 0 || time.Duration(*rd.ShortFor)*time.Second < RadioShortAfter {
			continue
		}
		ago = rd.ShortFor
		if *rd.BSS == 0 {
			add(Critical, "radio-down", rd.Band, bandName(rd.Band)+" off the air", fmt.Sprintf("its %s radio (%s) has none of its %d networks on the air: their clients have no %s service here",
				bandName(rd.Band), rd.Radio, *rd.BSSWant, bandName(rd.Band)))
			continue
		}
		which := ""
		if len(rd.Missing) > 0 {
			which = " (" + strings.Join(rd.Missing, ", ") + ")"
		}
		add(Warning, "radio-short", rd.Band, fmt.Sprintf("%d %s networks off the air", *rd.BSSWant-*rd.BSS, bandName(rd.Band)), fmt.Sprintf("%d of its %d networks on %s are not on the air%s: their clients have no %s service here",
			*rd.BSSWant-*rd.BSS, *rd.BSSWant, bandName(rd.Band), which, bandName(rd.Band)))
	}
	// A radio whose beacons do not go out (0221): its clients count the
	// beacons they miss, and leave an AP after a few in a row. Over the
	// last half hour (0339), not a report's minutes.
	for _, b := range in.Beacons {
		if _, on := BeaconsJudged(b); !on {
			continue
		}
		over := fmt.Sprintf("%d seconds", b.Over)
		if b.Over >= 120 {
			over = fmt.Sprintf("%d minutes", (b.Over+30)/60)
		}
		ago = nil
		add(Warning, "beacons", b.Band, bandName(b.Band)+" beacons not going out", fmt.Sprintf("%.1f%% of its beacons on %s did not go out over the last %s (%d of %d): its clients may lose it. The channel is too busy to send on, or the radio is stuck",
			100*float64(b.Failed)/float64(b.Sent+b.Failed), bandName(b.Band), over, b.Failed, b.Sent+b.Failed))
	}
	// The wire as a choke point (0351): a port that links slower than both
	// ends offer, or at half duplex; and an uplink port that ran full, had
	// errors, or lost its link, by its minutes.
	for _, p := range r.Ports {
		m := linkSpeedRE.FindStringSubmatch(p.Speed)
		if !p.Up || m == nil {
			continue
		}
		speed, _ := strconv.Atoi(m[1])
		if m[2] == "H" {
			add(Warning, "link-half", p.Name, p.Name+" at half duplex", fmt.Sprintf("%s runs at half duplex, at %s: it and the switch disagree on how to talk, and their frames collide. Have both ends negotiate", p.Name, linkRate(speed)))
		}
		if p.Max != nil && p.PartnerMax != nil && speed > 0 && speed < min(*p.Max, *p.PartnerMax) {
			best := min(*p.Max, *p.PartnerMax)
			add(Warning, "link-slow", p.Name, fmt.Sprintf("%s at %s, not %s", p.Name, linkRate(speed), linkRate(best)), fmt.Sprintf("%s links at %s, though it and the switch both offer %s: a cable, a connector or the switch port holds it back", p.Name, linkRate(speed), linkRate(best)))
		}
	}
	for _, u := range uplinkTrouble(in.Uplink, in.Now) {
		if u.Full >= UplinkFullMinutes {
			way := "into the AP, to its clients"
			if u.PeakOut {
				way = "out of the AP, from its clients"
			}
			add(Warning, "uplink-full", u.Port, "uplink "+u.Port+" full", fmt.Sprintf("%s, its %s uplink, ran full in %d minutes of the last %d, at %s at its busiest, %s: there the wire, not the Wi-Fi, held its clients back",
				u.Port, linkRate(u.Speed), u.Full, int(UplinkBusyWindow/time.Minute), mbitRate(u.Peak), way))
		}
		if u.Errors >= UplinkErrorsFewest {
			add(Warning, "uplink-errors", u.Port, fmt.Sprintf("uplink %s: %d errors", u.Port, u.Errors), fmt.Sprintf("%s, its uplink, had %d errors in the last %d minutes: a cable, a connector or the switch port is at fault, and what they spoil is sent again",
				u.Port, u.Errors, int(UplinkBusyWindow/time.Minute)))
		}
		if u.Losses >= UplinkLossesFewest {
			add(Warning, "uplink-flaps", u.Port, fmt.Sprintf("uplink %s lost its link %d times", u.Port, u.Losses), fmt.Sprintf("%s, its uplink, lost its link %d times in the last hour: each time, its clients reach nothing until it is back. A cable, a connector, the switch port, or its power",
				u.Port, u.Losses))
		}
	}
	// An update that failed is news only while the bundle it was to is
	// still the one the AP should run, and it does not run it (0079).
	if u := r.Agent; u != nil && u.Update != nil && (u.Update.State == "failed" || u.Update.State == "rolled-back") &&
		in.WantsAgent != "" && u.Update.Hash == in.WantsAgent && u.Hash != in.WantsAgent {
		ago = u.Update.Ago
		add(Warning, "agent-update", u.Update.Version, "agent update "+u.Update.State, fmt.Sprintf("the agent update to %s %s: %s", u.Update.Version, u.Update.State, u.Update.Why))
	}
	for _, net := range sortedKeys(r.Transports) {
		t := r.Transports[net]
		switch {
		case t.Active == "none":
			add(Critical, "no-transport", net, net+" has no transport", fmt.Sprintf("network %s has no transport in its bridge: its clients reach nothing", net))
		case t.CannotSwitch != "":
			add(Warning, "cannot-switch", net, net+" cannot switch", fmt.Sprintf("network %s cannot switch: %s", net, t.CannotSwitch))
		case t.Active == "fallback":
			add(Warning, "on-fallback", net, net+" on its fallback", fmt.Sprintf("network %s runs on its fallback: its primary is down", net))
		}
	}
	// A network that went off the air on this AP while it cannot reach its
	// way on, as it was set to (0201).
	for _, net := range sortedKeys(r.OffAir) {
		if o := r.OffAir[net]; o.Off {
			why := o.Why
			if why == "" {
				why = "its way on does not answer"
			}
			ago = o.Ago
			add(Warning, "off-air", net, net+" off the air here", fmt.Sprintf("network %s is off the air on this AP: %s, so its clients join another AP or network until it answers", net, why))
		}
	}
	// A radio whose firmware crashed (0310), the latest crash of each
	// device: one that did not come back stays down until its driver is
	// bound again, which a restart of the AP does, unless it serves again;
	// one that did dropped its clients as it restarted, and is said for
	// FWCrashFor.
	latest := map[string]int{}
	for i, c := range r.FWCrashes {
		latest[c.Device] = i
	}
	for _, dev := range sortedKeys(latest) {
		c := r.FWCrashes[latest[dev]]
		if c.Ago == nil {
			continue
		}
		what, part := "a radio ("+c.Driver+" "+c.Device+")", "a radio"
		if len(c.Bands) > 0 {
			var bs []string
			for _, b := range c.Bands {
				bs = append(bs, bandName(b))
			}
			part = andList(bs) + " radio"
			if len(c.Bands) > 1 {
				part += "s"
			}
			what = "its " + part + " (" + strings.Join(c.Radios, ", ") + ")"
		}
		why := ""
		if c.Why != nil && *c.Why != "" {
			why = " (" + *c.Why + ")"
		}
		// The scan radio (0126) serves no client: its crash is a warning,
		// and it is over only with the AP's restart, as it serves nothing.
		if c.Scan {
			what, part = "its scan radio ("+strings.Join(c.Radios, ", ")+")", "scan radio"
			ago = c.Ago
			if !c.Recovered && starting(c.Lost, *c.Ago) {
				add(Warning, "fw-crash", dev, part+" firmware starting again", fmt.Sprintf("the firmware of %s crashed%s and is starting again", what, why))
			} else if !c.Recovered {
				add(Warning, "fw-crash", dev, part+" firmware down", fmt.Sprintf("the firmware of %s crashed%s and it did not come back: its surveys and client tests stop until a restart of the AP", what, why))
			} else if time.Duration(*c.Ago)*time.Second < FWCrashFor {
				add(Warning, "fw-crash", dev, part+" firmware crashed, back", fmt.Sprintf("the firmware of %s crashed%s and came back", what, why))
			} else {
				ago = nil
			}
			continue
		}
		// One lost is over once a radio of it serves again: its driver was
		// bound again by hand, without a restart. Of one not lost, hostapd
		// may say its networks are on the air while its firmware is down,
		// as the SoC's own radios' while their processor waits (the W-318,
		// 2026-11-05): the agent's word, which counts its failed commands,
		// is it.
		back := c.Recovered
		for _, rd := range r.Radios {
			back = back || (c.Lost && slices.Contains(c.Radios, rd.Radio) && rd.BSS != nil && *rd.BSS > 0)
		}
		switch {
		case !back && starting(c.Lost, *c.Ago):
			ago = c.Ago
			add(Warning, "fw-crash", dev, part+" firmware starting again", fmt.Sprintf("the firmware of %s crashed%s and is starting again: its clients are dropped meanwhile", what, why))
		case !back && c.Driver == "remoteproc":
			// The SoC's own radios (AHB): their processor starts again by
			// itself, and hostapd may say they are on the air meanwhile.
			ago = c.Ago
			add(Critical, "fw-crash", dev, part+" firmware down", fmt.Sprintf("the firmware of %s crashed%s and has not started again: its clients have no service here meanwhile, whatever hostapd says. "+
				"Its processor starts again by itself, in up to 5 minutes where the build waits for its crash dump to be read; if it does not, a restart of the AP brings it back", what, why))
		case !back:
			ago = c.Ago
			add(Critical, "fw-crash", dev, part+" firmware down", fmt.Sprintf("the firmware of %s crashed%s and it did not come back: it stays down until its driver is bound again, which a restart of the AP does", what, why))
		case time.Duration(*c.Ago)*time.Second < FWCrashFor:
			ago = c.Ago
			add(Warning, "fw-crash", dev, part+" firmware crashed, back", fmt.Sprintf("the firmware of %s crashed%s and came back: its clients were dropped as it started again", what, why))
		}
	}
	// A process that holds two builds of a library at once (0317): one was
	// installed for another image than the AP's, and a program with both
	// may wait for good, as three agents did on 2026-11-05.
	for _, m := range r.MixedLibs {
		hold := "hold"
		if len(m.Procs) == 1 {
			hold = "holds"
		}
		add(Warning, "mixed-libs", m.Lib, "two builds of "+m.Lib, fmt.Sprintf("%s %s two builds of %s at once (%s): a package was installed for another image than the AP's, and what holds both may stop working without a word, as three agents did on 2026-11-05. Put in the AP's own build of it, and start them again",
			andList(m.Procs), hold, m.Lib, strings.Join(m.Builds, ", ")))
	}
	out = append(out, minutesStopped(in, st, r)...)
	// An address the AP holds on a network its own Wi-Fi serves (0209):
	// what the AP itself broadcasts there goes out over the air, and that
	// network's clients can reach it.
	for _, a := range r.OnAir {
		if len(a.SSIDs) == 0 {
			continue
		}
		where := "on a bridge"
		if a.VLAN != nil {
			where = fmt.Sprintf("on VLAN %d", *a.VLAN)
		}
		if a.Management {
			add(Warning, "shared-with-wifi", a.Interface, "managed on a Wi-Fi network", fmt.Sprintf("managed %s (%s), which %s also on: what the AP itself broadcasts goes out over the air, and their clients can reach its management. Manage it on a VLAN no Wi-Fi network is on",
				where, a.Interface, ssidsOn(a.SSIDs)))
			continue
		}
		add(Warning, "shared-with-wifi", a.Interface, "an address on a Wi-Fi network", fmt.Sprintf("holds an address %s (%s), which %s also on: what the AP itself broadcasts there goes out over the air, and their clients can reach it there. Take the address off, or move it to a VLAN no Wi-Fi network is on",
			where, a.Interface, ssidsOn(a.SSIDs)))
	}
	if x := r.VXLAN; x != nil {
		for _, t := range x.Tunnels {
			// Up to netifd, but its device gone (0187): nothing goes over
			// it, until the prober makes it again.
			if t.Gone {
				add(Critical, "tunnel-gone", fmt.Sprint(t.VNI), fmt.Sprintf("tunnel %d has no device", t.VNI), fmt.Sprintf("tunnel VNI %d to %s has no device, though netifd has it up: its clients reach nothing", t.VNI, t.Peer))
				continue
			}
			if !t.Standby && t.Probe != nil && t.Probe.Verdict == "down" {
				add(Warning, "tunnel-down", fmt.Sprint(t.VNI), fmt.Sprintf("tunnel %d down", t.VNI), fmt.Sprintf("tunnel VNI %d to %s is down", t.VNI, t.Peer))
			}
		}
		for _, l := range x.Loops {
			ago = l.Ago
			add(Critical, "loop", fmt.Sprintf("%s/%d", l.Port, l.VNI), "port "+l.Port+" loops", fmt.Sprintf("port %s loops on VNI %d: the loop guard took it off its tunnels", l.Port, l.VNI))
		}
	}
	for _, v := range r.UplinkVLANs {
		if v.Verdict == "silent" {
			add(Warning, "vlan-silent", fmt.Sprint(v.VLAN), fmt.Sprintf("VLAN %d silent on the uplink", v.VLAN), fmt.Sprintf("VLAN %d is silent on the uplink: the switch port may not carry it", v.VLAN))
		}
	}
	for _, net := range sortedKeys(r.DHCP) {
		// Two requests unanswered at the least: one alone, as a client's
		// lease renewal the server let pass, is answered at its next try
		// (Griff, 2026-10-23: two ESP32s raised 30 alerts in three days so,
		// and stayed online).
		if d := r.DHCP[net]; d.Unanswered >= minUnanswered && d.Answered == 0 {
			add(Warning, "dhcp-silent", net, "no DHCP answer on "+net, fmt.Sprintf("DHCP on network %s: %d requests in the last 10 minutes, and nothing answered", net, d.Unanswered))
		}
		// More than one DHCP server answered on the network in the last
		// hour (0160): one is a rogue, or set up by mistake. Where the
		// network says the servers it has, any other is, one alone too. A
		// server heard with no address of its own is the one of its MAC.
		d := r.DHCP[net]
		d.Servers = FoldDHCP(d.Servers, func(s dhcpHeard) string { return s.ID }, func(s dhcpHeard) string { return s.MAC }, nil)
		if len(in.DHCPServers[net]) > 0 {
			var others []string
			for _, x := range d.Servers {
				if !DHCPKnown(in.DHCPServers[net], x.ID, x.MAC) {
					others = append(others, fmt.Sprintf("%s (%s)", x.ID, x.MAC))
				}
			}
			if len(others) > 0 {
				add(Warning, "dhcp-servers", net, "another DHCP server on "+net, fmt.Sprintf("a DHCP server that is not one of network %s's answers there: %s. A rogue, or one set up by mistake; if it is the network's own, add it to the network's dhcp_servers", net, strings.Join(others, ", ")))
			}
		} else if len(d.Servers) > 1 {
			var each []string
			for _, x := range d.Servers {
				each = append(each, fmt.Sprintf("%s (%s)", x.ID, x.MAC))
			}
			add(Warning, "dhcp-servers", net, fmt.Sprintf("%d DHCP servers on %s", len(d.Servers), net), fmt.Sprintf("%d DHCP servers answer on network %s: %s. One is a rogue, or set up by mistake; if they are both the network's own, as a failover pair, say so in its dhcp_servers", len(d.Servers), net, strings.Join(each, ", ")))
		}
	}
	// Its scan radio (0126), which radio resource management listens through
	// and client tests borrow: its service not answering, no radio for it,
	// or a radio that hears nothing and that nothing is bringing back. One
	// airscan is binding again, or has just, is no alert: it sees to it.
	// heal_failed counts the binds that failed running, and is none again
	// once one works: more than none, the latest did not bring it back.
	if sr := r.ScanRadio; sr != nil {
		switch {
		case sr.State == "missing":
			add(Warning, "scan-radio", "", "airscan does not answer", "airscan, which runs its scan radio, does not answer: channel ratings age and client tests wait until it does")
		case sr.State == "waiting":
			add(Warning, "scan-radio", "", "no scan radio", "its scan radio is not there for airscan to use: channel ratings age and client tests wait until it is")
		case sr.Networks != nil && *sr.Networks == 0 && sr.LookedAgo != nil && *sr.LookedAgo <= 900 && sr.State != "healing" &&
			(sr.Heals == nil || (sr.HealFailed != nil && *sr.HealFailed > 0)):
			why := "and its airscan does not bring a deaf radio back: bind its driver again on the AP, or update airscan"
			if sr.Heals != nil {
				why = "and binding its driver again did not bring it back"
			}
			add(Warning, "scan-radio", "", "scan radio hears nothing", "its scan radio hears no network, not even its own, "+why)
		}
	}
	// The AP itself (0119): memory or storage nearly gone, a processor
	// busy all through the time since its last report, or running hot.
	if h := r.Health; h != nil {
		if h.MemTotal > 0 {
			switch free := 100 * h.MemAvailable / h.MemTotal; {
			case free < 5:
				add(Critical, "memory-low", "", fmt.Sprintf("memory low: %d%% free", free), fmt.Sprintf("%d%% of its memory is free: %d MB of %d", free, h.MemAvailable/1024, h.MemTotal/1024))
			case free < 10:
				add(Warning, "memory-low", "", fmt.Sprintf("memory low: %d%% free", free), fmt.Sprintf("%d%% of its memory is free: %d MB of %d", free, h.MemAvailable/1024, h.MemTotal/1024))
			}
		}
		if h.StorageTotal > 0 && 100*h.StorageFree/h.StorageTotal < 5 {
			add(Warning, "storage-low", "", fmt.Sprintf("storage low: %d%% free", 100*h.StorageFree/h.StorageTotal), fmt.Sprintf("%d%% of its storage is free: %d kB of %d; a config or an update may not fit", 100*h.StorageFree/h.StorageTotal, h.StorageFree, h.StorageTotal))
		}
		if h.CPU != nil && *h.CPU >= 90 {
			add(Warning, "cpu-busy", "", fmt.Sprintf("processor %d%% busy", *h.CPU), fmt.Sprintf("its processor was %d%% busy over the time since its last report", *h.CPU))
		}
		if h.Temp != nil && *h.Temp >= 95 {
			add(Warning, "hot", "", fmt.Sprintf("hot: %.0f °C", *h.Temp), fmt.Sprintf("it runs at %.0f °C", *h.Temp))
		}
	}
	// A RADIUS server that does not answer this AP (0114): no one can sign
	// in to the network through it.
	for _, x := range r.Radius {
		if x.Verdict == "silent" {
			add(Critical, "radius-silent", x.Network, "RADIUS silent for "+x.Network, fmt.Sprintf("the RADIUS server %s does not answer this AP: no one can sign in to network %s here. It is down, cannot be reached, or does not know this AP or its secret", x.Server, x.Network))
		}
	}
	// A network not one of the APs' that the scan radio found on our wire
	// is a rogue for certain (0160). One broadcasting one of Aeolus's SSIDs
	// and not found there is an evil twin, or an AP of the same name Aeolus
	// does not manage (0105).
	if f := in.Fleet; f != nil {
		heard := r.heard()
		// Each named by its maker, as the threats page names it (0160): the
		// registry's for its prefix, or a sibling BSSID's heard with it.
		bssids := make([]string, 0, len(heard))
		for _, o := range heard {
			bssids = append(bssids, o.BSSID)
		}
		by := func(bssid string) string {
			if m, _ := oui.BSSIDMaker(bssid, bssids); m != "" {
				return bssid + " (" + m + ")"
			}
			return bssid
		}
		for _, o := range heard {
			if f.Own(o.BSSID) || in.Known[o.BSSID] {
				continue
			}
			switch {
			case o.Wired:
				ago = o.Ago
				add(Critical, "rogue-wired", o.BSSID, "rogue AP on your wire: "+named(o.SSID), fmt.Sprintf("%s, not one of the APs', is on your wire, on %s: frames sent there came out of its radio. It broadcasts %s on %s channel %d, heard at %.0f dBm",
					by(o.BSSID), vlanList(o.VLANs), named(o.SSID), bandName(o.Band), o.Channel, o.Signal))
			case f.SSIDs[o.SSID]:
				ago = o.Ago
				add(Warning, "rogue", o.BSSID, fmt.Sprintf("evil twin of %q", o.SSID), fmt.Sprintf("%s, not one of the APs', broadcasts %q on %s channel %d, heard at %.0f dBm: an evil twin, or an AP of that name Aeolus does not manage",
					by(o.BSSID), o.SSID, bandName(o.Band), o.Channel, o.Signal))
			}
		}
	}
	// RRM with nowhere to go (0267): a band whose channels it rated, none
	// of them its best. A radio's own block is always one it may go to, so
	// none means RRM finds no block it may use at all, and moves it nowhere.
	if r.RRM != nil {
		rated, best := map[string]int{}, map[string]bool{}
		for _, x := range r.RRM.Ratings {
			// One only remembered (0332) is no rating RRM moves by.
			if x.Stale {
				continue
			}
			rated[x.Band]++
			best[x.Band] = best[x.Band] || x.Best
		}
		// Not a band no radio here has a network on, where the report says
		// its radios: RRM's ratings of it are left from before, as the
		// W-318's 6 GHz were once a test network there was taken off
		// (2026-11-05), and it moves no radio that serves nothing.
		serves := func(band string) bool {
			if len(r.Radios) == 0 {
				return true
			}
			for _, rd := range r.Radios {
				if rd.Band == band && ((rd.BSS != nil && *rd.BSS > 0) || (rd.BSSWant != nil && *rd.BSSWant > 0)) {
					return true
				}
			}
			return false
		}
		for _, b := range sortedKeys(rated) {
			if rated[b] >= nowhereRated && !best[b] && serves(b) {
				add(Warning, "rrm-nowhere", b, "RRM finds no "+bandName(b)+" channel", fmt.Sprintf("RRM rated %d of its %s channels and finds none its radio may use, not even its own: it moves it nowhere. Its radio's channels and width may leave it no whole block, or RRM reads them wrong (0266)",
					rated[b], bandName(b)))
			}
		}
	}
	// A radio that can't scan, past the half hour its ratings last (0263):
	// RRM rates its channels no more, so it neither moves it nor sees a
	// neighbour come. One on a DFS channel it went to after a look (0120)
	// has where it steps off to, and rates its own channel by its counts.
	if r.RRM != nil {
		for _, d := range r.RRM.CannotScan {
			if d.Home > 0 || d.Ago < deafFor {
				continue
			}
			short := bandName(d.Band) + " radio cannot scan"
			msg := fmt.Sprintf("its %s radio has refused every scan for %s, on channel %d, so RRM rates no channel for it and moves it nowhere",
				bandName(d.Band), lasted(d.Ago), d.Channel)
			if d.Why == "dfs" {
				short = fmt.Sprintf("%s cannot scan on DFS %d", bandName(d.Band), d.Channel)
				msg = fmt.Sprintf("its %s radio has been on DFS channel %d for %s, where Linux keeps it listening for radar and refuses every scan, so RRM rates no channel for it and moves it nowhere. With radio.%s.dfs allow, a radio starts outside DFS and goes to one only after a look where RRM moves radios and its width and channels leave it a block outside DFS to start on: 160 MHz has none on 5 GHz (0120, 0328)",
					bandName(d.Band), d.Channel, lasted(d.Ago), d.Band)
			}
			a := d.Ago
			ago = &a
			add(Warning, "cannot-scan", d.Band, short, msg)
		}
	}
	// Power control that asks (0206): a client only this AP reaches that
	// hears it below the threshold set, or clients that took a step down
	// badly in the last hour. Either is for a person to choose: guard, and
	// power control keeps such clients in reach, or ignore, and it steps by
	// the neighbours alone. Where it guards or ignores, it has its answer.
	if r.RRM != nil {
		for _, p := range r.RRM.APC {
			if p.Clients != "" && p.Clients != "ask" {
				continue
			}
			floor := -80
			if p.ClientFloor != nil {
				floor = *p.ClientFloor
			}
			var below []string
			for _, c := range p.Lonely {
				if c.Est < floor {
					below = append(below, c.MAC)
				}
			}
			if len(below) > 0 {
				// What it hears is what it said (802.11k), or reckoned.
				about, says := "about ", ""
				if p.Lonely[0].Measured {
					about, says = "", ", as it says itself"
				}
				add(Warning, "apc-client", p.Band, bandName(p.Band)+" power leaves a client weak", fmt.Sprintf("power control has %s at %d dBm, where %s, which no other AP hears better, hears it at %s%d dBm%s, below %d. Choose apc.clients guard to keep such clients in reach, or ignore to step by the neighbours alone",
					bandName(p.Band), p.Power, clientsSaid(below), about, p.Lonely[0].Est, says, floor))
			}
			if d := p.Distress; d != nil && !d.Acted && d.At > 0 && st.At.Sub(time.Unix(d.At, 0)) < time.Hour {
				a := int(max(0, st.At.Unix()-d.At))
				ago = &a
				add(Warning, "apc-distress", p.Band, bandName(p.Band)+" power step hurt clients", fmt.Sprintf("after power control stepped %s down, its clients %s. Choose apc.clients guard to step back and hold, or ignore to keep stepping by the neighbours alone",
					bandName(p.Band), distressSaid(d.Why, d.Scanning, d.Left, d.Ratio)))
			}
		}
	}
	if t := r.Time; t != nil && t.Synced != nil && !*t.Synced {
		add(Info, "clock", "", "clock not synced", "the clock is not synced: logs and key expiry may be off")
	}
	// What runs by local hours with no zone set runs by UTC's (0262): what
	// the AP runs, where its own clock is on UTC; what the manager runs for
	// it, at once.
	if in.Zone == "" {
		var utc []string
		if t := r.Time; t != nil && t.Rule != nil && utcRule.MatchString(*t.Rule) {
			utc = append(utc, in.OnClock...)
		}
		utc = append(utc, in.ByZone...)
		if len(utc) > 0 {
			add(Warning, "clock-zone", "", "no time zone set", fmt.Sprintf("no time zone is set for it, so %s happen at those hours in UTC. Set system.tz on its folder",
				andList(utc)))
		}
	}
	return out
}

// nowhereRated is how many of a band's channels RRM must have rated
// before having no best among them is said (0267).
const nowhereRated = 4

// deafFor is how long a radio can't scan before it is said (0263): its
// ratings last half an hour (0073).
const deafFor = 30 * 60

// lasted says seconds the way people say them, as ago does.
// minutesStopped is the alert of an AP whose radios serve networks and
// whose minutes do not come (0311): of all its radios, where it sent none
// for MinutesQuiet; else of each radio that serves networks and sent none
// for as long, while the others' come. Only of an AP that calls, has been
// up MinutesQuiet, and sent minutes before: an AP's agent sends them from
// its start. A radio is quiet from its newest minute's end, and not before
// the AP started, nor, as one its config may have just turned on, before
// its current version was made, nor MinutesLooked ago.
func minutesStopped(in Input, st *conditions.State, r report) []Alert {
	m := in.Minutes
	if m == nil || m.Last.IsZero() || r.Uptime == nil || in.Latest.Seen == nil || in.Now.Sub(in.Latest.Seen.At) > Offline(in.Poll) {
		return nil
	}
	boot := st.At.Add(-time.Duration(*r.Uptime) * time.Second)
	if in.Now.Sub(boot) < MinutesQuiet {
		return nil
	}
	alert := func(key, short, msg string, since time.Time) Alert {
		return Alert{AP: in.AP, Name: in.Name, Severity: Warning, Kind: "minutes-stopped", Key: key, Short: short, Message: msg, Since: &since}
	}
	serves := 0
	for _, rd := range r.Radios {
		if rd.BSS != nil && *rd.BSS > 0 {
			serves++
		}
	}
	if serves == 0 {
		return nil
	}
	const lost = "Aeolus has no charts, power history, client journeys or struggling alerts of it since"
	if end := m.Last.Add(time.Minute); in.Now.Sub(end) >= MinutesQuiet {
		return []Alert{alert("minutes-stopped", "no minutes for "+ago(in.Now.Sub(end)), fmt.Sprintf("has sent no minute of its radios for %s, though it calls and serves networks: %s. Its agent starts the sampler (aeolus-live) again after 5 minutes without one (0307), and that has not brought them back",
			ago(in.Now.Sub(end)), lost), end)}
	}
	var out []Alert
	for _, rd := range r.Radios {
		if rd.BSS == nil || *rd.BSS == 0 {
			continue
		}
		since := latestOf(boot, in.Since, in.Now.Add(-MinutesLooked))
		if t, ok := m.Radios[rd.Radio]; ok {
			since = latestOf(since, t.Add(time.Minute))
		}
		if in.Now.Sub(since) < MinutesQuiet {
			continue
		}
		nets := fmt.Sprintf("%d networks", *rd.BSS)
		if *rd.BSS == 1 {
			nets = "a network"
		}
		out = append(out, alert("minutes-stopped:"+rd.Radio, bandName(rd.Band)+" sends no minutes", fmt.Sprintf("its %s radio (%s) has sent no minute for %s, though it serves %s and its other radios' minutes come: %s",
			bandName(rd.Band), rd.Radio, ago(in.Now.Sub(since)), nets, lost), since))
	}
	return out
}

// latestOf is the latest of times.
func latestOf(times ...time.Time) time.Time {
	var out time.Time
	for _, t := range times {
		if t.After(out) {
			out = t
		}
	}
	return out
}

func lasted(secs int) string { return ago(time.Duration(secs) * time.Second) }

// utcRule is a POSIX TZ rule that keeps UTC all year: a name, or none, with
// no offset or one of zero, and no summer time; or no rule at all (0262).
var utcRule = regexp.MustCompile(`^(<[^>]*>|[A-Za-z]{3,})?([+-]?0{1,2}(:00){0,2})?$`)

// clientsSaid names lonely clients: one, two, or the first and how many
// more.
func clientsSaid(macs []string) string {
	switch len(macs) {
	case 1:
		return macs[0]
	case 2:
		return macs[0] + " and " + macs[1]
	}
	return fmt.Sprintf("%s and %d more", macs[0], len(macs)-1)
}

// distressSaid says how clients took a step down badly (0206).
func distressSaid(why string, scanning, left int, ratio []*float64) string {
	switch why {
	case "scanning":
		return fmt.Sprintf("began looking for another AP: %d of them sent many more probe requests", scanning)
	case "leaving":
		return fmt.Sprintf("began to leave: %d left or came back", left)
	case "retries":
		if len(ratio) == 2 && ratio[0] != nil && ratio[1] != nil {
			return fmt.Sprintf("needed more tries: %.0f%% of what was sent to them was tried again or lost, from %.0f%%", *ratio[1]*100, *ratio[0]*100)
		}
		return "needed more tries"
	}
	return "did worse"
}

// speedPair says a speed each way, down/up in Mbit/s to a tenth, a dash
// for a way not measured.
func speedPair(down, up *float64) string {
	mbit := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return strconv.FormatFloat(math.Round(*v*10)/10, 'f', -1, 64)
	}
	return mbit(down) + "/" + mbit(up)
}

// andList says a list as a sentence does: "a", "a and b", "a, b and c".
func andList(each []string) string {
	if len(each) < 2 {
		return strings.Join(each, "")
	}
	return strings.Join(each[:len(each)-1], ", ") + " and " + each[len(each)-1]
}

// vlanList says which VLANs: "VLAN 10", "VLANs 10 and 20", or "a VLAN"
// where none is said.
func vlanList(vs []int) string {
	switch len(vs) {
	case 0:
		return "a VLAN"
	case 1:
		return fmt.Sprintf("VLAN %d", vs[0])
	}
	each := make([]string, len(vs))
	for i, v := range vs {
		each[i] = fmt.Sprint(v)
	}
	return "VLANs " + strings.Join(each[:len(each)-1], ", ") + " and " + each[len(each)-1]
}

// ssidsOn names the Wi-Fi networks on a network, with the verb: "Sweet
// Spot" is, "A" and "B" are, "A", "B" and 3 more are.
func ssidsOn(ssids []string) string {
	each := make([]string, len(ssids))
	for i, s := range ssids {
		each[i] = named(s)
	}
	switch len(each) {
	case 1:
		return each[0] + " is"
	case 2, 3:
		return strings.Join(each[:len(each)-1], ", ") + " and " + each[len(each)-1] + " are"
	}
	return fmt.Sprintf("%s, %s and %d more are", each[0], each[1], len(each)-2)
}

// whoName is a client as a short says it (0330): the name a person gave
// it, without the MAC that follows it in who, or its MAC where it has none.
func whoName(who, mac string) string {
	if n := strings.TrimSuffix(who, " ("+mac+")"); n != "" {
		return n
	}
	return mac
}

// named is an SSID as a message says it: quoted, or a hidden name.
func named(ssid string) string {
	if ssid == "" {
		return "a hidden name"
	}
	return fmt.Sprintf("%q", ssid)
}

func seenAt(s *conditions.Seen) *time.Time {
	if s == nil {
		return nil
	}
	return &s.At
}

// FWCrashFor is how long a radio's firmware crash it came back from is said
// (0310), and FWCrashStarting how long one it has not come back from yet,
// and that mac80211 has not said it lost, is taken to be starting again:
// a driver that recovers does so in seconds (ath11k with porting's 0085,
// 2.8 s), and the agent sees a serving radio back only a minute on.
const (
	FWCrashFor      = 6 * time.Hour
	FWCrashStarting = 2 * time.Minute
)

// starting says whether a crash not seen to have come back is still
// taken to be starting again: young, and not said to be lost.
func starting(lost bool, ago int) bool {
	return !lost && time.Duration(ago)*time.Second < FWCrashStarting
}

// ago writes a duration the way people say it: 12 minutes, 3 hours, 2 days.
func ago(d time.Duration) string {
	switch {
	case d < 2*time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < 2*time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// linkSpeedRE is a port's speed as the agent says it (0053): Mbit/s, then
// F for full duplex or H for half.
var linkSpeedRE = regexp.MustCompile(`^([0-9]+)([FH])$`)

// linkRate says a link's speed, given in Mbit/s.
func linkRate(mbit int) string {
	if mbit >= 1000 {
		return strconv.FormatFloat(float64(mbit)/1000, 'f', -1, 64) + " Gbit/s"
	}
	return strconv.Itoa(mbit) + " Mbit/s"
}

// mbitRate says a rate, given in Mbit/s: to a tenth of a Gbit/s from 1000.
func mbitRate(mbit float64) string {
	if mbit >= 1000 {
		return strconv.FormatFloat(math.Round(mbit/100)/10, 'f', -1, 64) + " Gbit/s"
	}
	return strconv.FormatFloat(math.Round(mbit), 'f', -1, 64) + " Mbit/s"
}

// uplinkSaid is what an uplink port's minutes came to (0351): over the last
// UplinkBusyWindow, how many minutes ran full, at its
// link's last Speed, the busiest it ran (Peak, in Mbit/s; PeakOut where
// that was out of the AP) and its errors; over the last UplinkLossWindow,
// how often it lost its link.
type uplinkSaid struct {
	Port    string
	Speed   int
	Full    int
	Peak    float64
	PeakOut bool
	Errors  int64
	Losses  int64
	changes int64
}

// uplinkTrouble is what each uplink port's minutes came to, by port.
func uplinkTrouble(ms []conditions.UplinkMinute, now time.Time) []uplinkSaid {
	by := map[string]*uplinkSaid{}
	var order []string
	for _, m := range ms {
		if m.At.Before(now.Add(-UplinkLossWindow)) {
			continue
		}
		u := by[m.Port]
		if u == nil {
			u = &uplinkSaid{Port: m.Port}
			by[m.Port] = u
			order = append(order, m.Port)
		}
		u.changes += m.Changes
		if m.At.Before(now.Add(-UplinkBusyWindow)) {
			continue
		}
		u.Errors += m.Errors
		if m.Speed == nil || *m.Speed <= 0 {
			continue
		}
		u.Speed = *m.Speed
		link := float64(*m.Speed)
		full := false
		for _, w := range []struct {
			avg, most *float64
			out       bool
		}{{m.Rx, m.RxMax, false}, {m.Tx, m.TxMax, true}} {
			if w.most != nil && *w.most > u.Peak {
				u.Peak, u.PeakOut = *w.most, w.out
			}
			if (w.most != nil && *w.most*100 >= link*UplinkFullPct) || (w.avg != nil && *w.avg*100 >= link*UplinkAvgPct) {
				full = true
			}
		}
		if full {
			u.Full++
		}
	}
	out := make([]uplinkSaid, 0, len(order))
	sort.Strings(order)
	for _, p := range order {
		u := by[p]
		// A link lost is two changes, down and up again.
		u.Losses = u.changes / 2
		out = append(out, *u)
	}
	return out
}

func bandName(b string) string {
	switch b {
	case "2g":
		return "2.4 GHz"
	case "5g":
		return "5 GHz"
	case "6g":
		return "6 GHz"
	}
	return b
}
