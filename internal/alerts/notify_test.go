package alerts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func events(es []Event) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Kind+":"+e.Alert.Key)
	}
	return strings.Join(out, " ")
}

func TestNotifierHoldsThenSendsOnceThenResolves(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Ntfy: "https://ntfy.example/topic", Resolved: true} }
	offline := Alert{AP: "ap-1", Name: "C360-AP", Severity: Critical, Kind: "offline", Key: "offline", Message: "offline"}
	t0 := now
	// The first look takes what is alerting as sent: nothing goes out.
	if got := events(n.Step(t0, nil, to)); got != "" {
		t.Fatalf("first look: %s", got)
	}
	if got := events(n.Step(t0.Add(time.Minute), []Alert{offline}, to)); got != "" {
		t.Fatalf("new, held: %s", got)
	}
	if got := events(n.Step(t0.Add(2*time.Minute), []Alert{offline}, to)); got != "" {
		t.Fatalf("held a minute: %s", got)
	}
	if got := events(n.Step(t0.Add(3*time.Minute), []Alert{offline}, to)); got != "alert:offline" {
		t.Fatalf("after the hold: %s", got)
	}
	if got := events(n.Step(t0.Add(4*time.Minute), []Alert{offline}, to)); got != "" {
		t.Fatalf("still alerting: %s", got)
	}
	if got := events(n.Step(t0.Add(5*time.Minute), nil, to)); got != "resolved:offline" {
		t.Fatalf("ended: %s", got)
	}
	// Back within the quiet time: nothing, however long it lasts.
	for m := 6; m <= 12; m++ {
		if got := events(n.Step(t0.Add(time.Duration(m)*time.Minute), []Alert{offline}, to)); got != "" {
			t.Fatalf("minute %d, quiet: %s", m, got)
		}
	}
}

func TestNotifierBlipIsNotSent(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Webhook: "https://hook.example/x"} }
	a := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	n.Step(now, nil, to)
	n.Step(now.Add(time.Minute), []Alert{a}, to)
	if got := events(n.Step(now.Add(2*time.Minute), nil, to)); got != "" {
		t.Fatalf("a blip: %s", got)
	}
	if got := events(n.Step(now.Add(5*time.Minute), []Alert{a}, to)); got != "" {
		t.Fatalf("back, held again: %s", got)
	}
}

func TestNotifierRestartSendsNothingAnew(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Webhook: "https://hook.example/x", Resolved: true} }
	a := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	if got := events(n.Step(now, []Alert{a}, to)); got != "" {
		t.Fatalf("first look: %s", got)
	}
	if got := events(n.Step(now.Add(10*time.Minute), []Alert{a}, to)); got != "" {
		t.Fatalf("already alerting at start: %s", got)
	}
	// Its end is news, and is sent.
	if got := events(n.Step(now.Add(11*time.Minute), nil, to)); got != "resolved:offline" {
		t.Fatalf("ended: %s", got)
	}
}

func TestTargetSeverity(t *testing.T) {
	warn := Alert{AP: "ap-1", Severity: Warning, Kind: "tunnel-down", Key: "tunnel-down:50"}
	n := NewNotifier()
	n.Step(now, nil, func(Alert) Target { return Target{} })
	for _, tc := range []struct {
		t    Target
		want string
	}{
		{Target{Ntfy: "https://n/x"}, ""}, // critical only, by default
		{Target{Ntfy: "https://n/x", Least: Warning}, "alert:tunnel-down:50"},
		{Target{Least: Warning}, ""}, // nowhere to send it
	} {
		n := NewNotifier()
		to := func(Alert) Target { return tc.t }
		n.Step(now, nil, to)
		n.Step(now.Add(time.Minute), []Alert{warn}, to)
		if got := events(n.Step(now.Add(4*time.Minute), []Alert{warn}, to)); got != tc.want {
			t.Fatalf("%+v: %s, want %s", tc.t, got, tc.want)
		}
	}
}

func TestSendWebhookAndNtfy(t *testing.T) {
	var hook map[string]any
	var ntfyBody string
	var ntfyHead http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/hook" {
			json.Unmarshal(b, &hook)
		} else {
			ntfyBody, ntfyHead = string(b), r.Header
		}
	}))
	defer srv.Close()
	e := Event{Kind: "alert", At: now, Target: Target{Webhook: srv.URL + "/hook", Ntfy: srv.URL + "/topic"},
		Alert: Alert{AP: "ap-1", Name: "C360-AP", Severity: Critical, Kind: "offline", Key: "offline", Message: "offline: last heard 6 minutes ago"}}
	if err := Send(context.Background(), srv.Client(), e); err != nil {
		t.Fatal(err)
	}
	if hook["event"] != "alert" || hook["alert"].(map[string]any)["name"] != "C360-AP" {
		t.Fatalf("webhook got %v", hook)
	}
	if ntfyBody != "offline: last heard 6 minutes ago" || ntfyHead.Get("Priority") != "5" || ntfyHead.Get("Title") != "Aeolus: C360-AP (critical)" {
		t.Fatalf("ntfy got %q %v", ntfyBody, ntfyHead)
	}
}

// An alert open at the first look that no target would have had is not
// taken as sent: an endpoint added later still hears of it.
func TestNotifierStartupOnlyMarksWhatWasDeliverable(t *testing.T) {
	n := NewNotifier()
	a := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	none := func(Alert) Target { return Target{} }
	n.Step(now, []Alert{a}, none)
	to := func(Alert) Target { return Target{Ntfy: "https://n/x"} }
	n.Step(now.Add(time.Minute), []Alert{a}, to)
	if got := events(n.Step(now.Add(4*time.Minute), []Alert{a}, to)); got != "alert:offline" {
		t.Fatalf("after an endpoint was added: %s", got)
	}
}

// Errors name only the target's origin, never its sealed path or query,
// and a redirect is refused, not followed as a GET without the alert.
func TestSendKeepsTheSecretOutOfErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/topic-secret", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	for _, path := range []string{"/topic-secret?auth=tok", "/moved"} {
		e := Event{Kind: "alert", Target: Target{Ntfy: srv.URL + path}, Alert: Alert{Name: "C360-AP", Severity: Critical, Message: "offline"}}
		err := Send(context.Background(), srv.Client(), e)
		if err == nil {
			t.Fatalf("%s: sent", path)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "tok") || strings.Contains(err.Error(), "moved") {
			t.Fatalf("%s: the error names the path: %v", path, err)
		}
	}
}

// An AP in maintenance (0218): nothing of it is sent; an alert that
// outlasts the maintenance is sent after the hold once it ends; one sent
// before it began is said to end, maintenance or not.
func TestNotifierMaintenance(t *testing.T) {
	n := NewNotifier()
	held := true
	to := func(Alert) Target {
		return Target{Ntfy: "https://ntfy.example/topic", Resolved: true, Maintenance: held}
	}
	offline := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	n.Step(now, nil, to)
	for m := 1; m <= 10; m++ {
		if got := events(n.Step(now.Add(time.Duration(m)*time.Minute), []Alert{offline}, to)); got != "" {
			t.Fatalf("minute %d, in maintenance: %s", m, got)
		}
	}
	held = false
	n.Step(now.Add(11*time.Minute), []Alert{offline}, to)
	if got := events(n.Step(now.Add(13*time.Minute), []Alert{offline}, to)); got != "alert:offline" {
		t.Fatalf("out of maintenance, after the hold: %s", got)
	}
	held = true
	if got := events(n.Step(now.Add(14*time.Minute), nil, to)); got != "resolved:offline" {
		t.Fatalf("one sent, ended in maintenance: %s", got)
	}
	if (Target{Webhook: "x", Maintenance: true}).Wants(Critical) {
		t.Fatal("a target in maintenance wants an alert")
	}
}

// An alert sent and ended that comes back saying it began when it did is
// the same episode (0306), and is not sent again, however long after the
// quiet time; one that says it began anew is.
func TestNotifierSameEpisodeNotSentAgain(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Ntfy: "https://ntfy.example/topic", Least: Warning} }
	since := now.Add(-10 * time.Hour)
	test := Alert{AP: "ap-1", Name: "Shed-AP", Severity: Warning, Kind: "client-test", Key: "client-test:enterprise:2g:Garage", Message: "failed at dhcp", Since: &since}
	t0 := now
	n.Step(t0, nil, to)
	n.Step(t0.Add(time.Minute), []Alert{test}, to)
	if got := events(n.Step(t0.Add(4*time.Minute), []Alert{test}, to)); got != "alert:client-test:enterprise:2g:Garage" {
		t.Fatalf("sent once: %s", got)
	}
	n.Step(t0.Add(5*time.Minute), nil, to)
	// An hour on, past the quiet time, the same episode: nothing.
	for m := 65; m <= 70; m++ {
		if got := events(n.Step(t0.Add(time.Duration(m)*time.Minute), []Alert{test}, to)); got != "" {
			t.Fatalf("minute %d, the same episode: %s", m, got)
		}
	}
	// It ends, and comes back as a new one: sent.
	n.Step(t0.Add(71*time.Minute), nil, to)
	later := t0.Add(90 * time.Minute)
	anew := test
	anew.Since = &later
	n.Step(t0.Add(90*time.Minute), []Alert{anew}, to)
	if got := events(n.Step(t0.Add(93*time.Minute), []Alert{anew}, to)); got != "alert:client-test:enterprise:2g:Garage" {
		t.Fatalf("a new episode: %s", got)
	}
}
