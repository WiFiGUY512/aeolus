package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

// Suggestions (0323): anyone signed in may ask Aeolus for a feature, from
// any page, with the page they were on. Each is kept on this manager
// first, so asking needs no internet, then sent on to where the manager is
// told to send them (WithSuggestTo), tried again until it gets there. What
// is sent on is what they wrote, how to reach them if they said, the page,
// the release and the Org: never their account, nor anything of clients.
const (
	suggestMax        = 4000 // the most a suggestion says, in characters
	suggestContactMax = 200
	suggestPageMax    = 300 // a page's route and heading are cut to this
	suggestPerHour    = 20  // the most one account asks in an hour
)

// WithSuggestTo sets where suggestions are sent on (0323): an http or https
// URL that takes a POST of each, as JSON. "" or "off" keeps them here.
func (s *Server) WithSuggestTo(to string) (*Server, error) {
	if to == "" || to == "off" {
		s.suggestTo = ""
		return s, nil
	}
	u, err := url.Parse(to)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return s, fmt.Errorf("suggestions go to an http or https URL, not %q", to)
	}
	s.suggestTo = to
	return s, nil
}

// suggestHost is where suggestions are sent on, as a person reads it: the
// host alone; "" where they are kept here.
func (s *Server) suggestHost() string {
	if s.suggestTo == "" {
		return ""
	}
	u, _ := url.Parse(s.suggestTo)
	return u.Host
}

// cut keeps at most n characters of a string, whole.
func cut(v string, n int) string {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) <= n {
		return v
	}
	return string([]rune(v)[:n])
}

// addSuggestion keeps what someone asks Aeolus to do, from the page they
// were on, and says where it goes on to.
func (s *Server) addSuggestion(w http.ResponseWriter, r *http.Request, c call) error {
	var req struct {
		Text    string `json:"text"`
		Contact string `json:"contact"`
		Route   string `json:"route"`
		Title   string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return badRequest("say what you would like Aeolus to do")
	}
	if utf8.RuneCountInString(text) > suggestMax {
		return badRequest("a suggestion says at most %d characters", suggestMax)
	}
	if utf8.RuneCountInString(strings.TrimSpace(req.Contact)) > suggestContactMax {
		return badRequest("how to reach you is at most %d characters", suggestContactMax)
	}
	n, err := s.conds.SuggestionsBy(string(c.actor), time.Now().Add(-time.Hour))
	if err != nil {
		return err
	}
	if n >= suggestPerHour {
		return &apiError{http.StatusTooManyRequests, fmt.Sprintf("%d suggestions an hour is the most: say it again later", suggestPerHour)}
	}
	// A page is one of this manager's own routes, #/…, or none: an admin
	// follows it from the list, so it is never another kind of link.
	route := cut(req.Route, suggestPageMax)
	if !strings.HasPrefix(route, "#/") {
		route = ""
	}
	x, err := s.conds.AddSuggestion(conditions.Suggestion{
		Account: string(c.actor), Text: text, Contact: strings.TrimSpace(req.Contact),
		Route: route, Title: cut(req.Title, suggestPageMax), Version: s.version,
	})
	if err != nil {
		return err
	}
	slog.Info("suggestion kept", "id", x.ID, "by", c.actor, "route", x.Route)
	writeJSON(w, http.StatusCreated, map[string]any{"suggestion": x, "to": s.suggestHost()})
	return nil
}

// suggestions lists the latest suggestions, for whoever views the Org's
// locations: its admins and its advisor.
func (s *Server) suggestions(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	if !may(c, change.Locations, t, t.Root(), access.View) {
		return &apiError{http.StatusForbidden, "the suggestions are read by those who view the Org"}
	}
	list, err := s.conds.Suggestions(200)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": list, "to": s.suggestHost()})
	return nil
}

// sentSuggestion is a suggestion as it is sent on (0323): enough to read it
// and to tell it from the next, and nothing of who asked but what they
// gave to be reached by. key is the same each time it is sent, so one sent
// twice is known.
type sentSuggestion struct {
	Kind    string    `json:"kind"`
	V       int       `json:"v"`
	Key     string    `json:"key"`
	Org     string    `json:"org"`
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Version string    `json:"version,omitempty"`
	Text    string    `json:"text"`
	Contact string    `json:"contact,omitempty"`
	Page    struct {
		Route string `json:"route,omitempty"`
		Title string `json:"title,omitempty"`
	} `json:"page"`
}

// Suggest sends on each suggestion not yet sent, every so often, while
// ctx lasts (0323). One that does not get there is tried again, a minute
// on and twice as long each time, up to six hours apart.
func (s *Server) Suggest(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		s.sendSuggestions(ctx, &http.Client{Timeout: 20 * time.Second})
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// sendSuggestions sends on what is due, with c.
func (s *Server) sendSuggestions(ctx context.Context, c *http.Client) {
	if s.suggestTo == "" {
		return
	}
	due, err := s.conds.SuggestionsDue(time.Now(), 20)
	if err != nil {
		slog.Warn("suggestions: cannot read what is due", "err", err)
		return
	}
	org := string(s.log.Snapshot().Org.Locations.Root())
	for _, x := range due {
		if err := s.sendSuggestion(ctx, c, org, x); err != nil {
			wait := time.Minute << min(x.Tries, 9)
			if wait > 6*time.Hour {
				wait = 6 * time.Hour
			}
			slog.Info("suggestion not sent yet", "id", x.ID, "err", err, "again_in", wait)
			s.conds.SuggestionFailed(x.ID, err.Error(), time.Now().Add(wait))
			continue
		}
		slog.Info("suggestion sent", "id", x.ID, "to", s.suggestHost())
		s.conds.SuggestionSent(x.ID, s.suggestHost())
	}
}

// sendSuggestion posts one suggestion. A redirect is refused rather than
// followed, as an alert's is (0101): followed, it would go as a GET without
// the suggestion and be called sent.
func (s *Server) sendSuggestion(ctx context.Context, c *http.Client, org string, x conditions.Suggestion) error {
	out := sentSuggestion{Kind: "aeolus-suggestion", V: 1, Org: org, ID: x.ID, At: x.At, Version: x.Version, Text: x.Text, Contact: x.Contact}
	out.Page.Route, out.Page.Title = x.Route, x.Title
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s %d %d", org, x.ID, x.At.UnixMilli())))
	out.Key = hex.EncodeToString(sum[:16])
	body, err := json.Marshal(out)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.suggestTo, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: the URL does not parse", s.suggestHost())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aeolus-suggestions")
	nr := *c
	nr.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := nr.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%s: %w", s.suggestHost(), err)
	}
	res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("%s answered %s", s.suggestHost(), res.Status)
	}
	return nil
}
