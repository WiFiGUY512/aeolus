package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/advisor"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// advising is the advisor's state in the manager (0205): what it reads
// with, and whose review is under way, by its folder (0212).
type advising struct {
	mu      sync.Mutex
	reads   func(http.Handler) *mcp.Server // the MCP adapter's server over a handler of the API; nil, no advisor
	running hierarchy.NodeID               // the folder whose review is under way, or none
	baseURL string                         // Anthropic's API's, for tests
}

// WithAdvisor gives the server the reads its advisor makes (0205): the MCP
// adapter's server over a handler of the API, which a review gives it as
// advisor.Account, held to its folder (0212).
func (s *Server) WithAdvisor(reads func(http.Handler) *mcp.Server) *Server {
	s.adv.reads = reads
	return s
}

// advisorSettings are a folder's advisor's (0212), as they resolve at its
// folder, the key opened.
type advisorSettings struct {
	Enabled bool
	KeySet  bool
	Every   int
	advisor.Settings
}

func (s *Server) advisorSettings(state *change.State, node hierarchy.NodeID) advisorSettings {
	out := advisorSettings{Every: 24, Settings: advisor.Settings{Scope: string(node), Model: "claude-opus-5-5", Web: true, Budget: 2}}
	t := state.Org.Locations
	if n, ok := t.Node(node); ok {
		out.ScopeName = n.Name
	}
	get := func(p string) (any, bool) {
		r, ok := t.Resolve(node, hierarchy.Path("system.advisor."+p))
		return r.Value, ok
	}
	if v, ok := get("enabled"); ok {
		out.Enabled, _ = v.(bool)
	}
	if v, ok := get("model"); ok {
		if m, _ := v.(string); m != "" {
			out.Model = m
		}
	}
	if v, ok := get("every"); ok {
		if n, ok := v.(float64); ok && n >= 1 {
			out.Every = int(n)
		}
	}
	if v, ok := get("web"); ok {
		out.Web, _ = v.(bool)
	}
	if v, ok := get("budget"); ok {
		if n, ok := v.(float64); ok && n > 0 {
			out.Budget = n
		}
	}
	if v, ok := get("key"); ok {
		out.KeySet = true
		if open, err := s.reveal("system.advisor.key", v); err == nil {
			out.Key, _ = open.(string)
		}
	}
	out.BaseURL = s.adv.baseURL
	return out
}

// review runs one review by the advisor of the folder node, unless one is
// under way, and says whether it began; with a question, one that answers
// it (0215). One runs at a time, whosever it is.
func (s *Server) review(ctx context.Context, node hierarchy.NodeID, why, question string) bool {
	if s.adv.reads == nil || s.conds == nil {
		return false
	}
	s.adv.mu.Lock()
	if s.adv.running != "" {
		s.adv.mu.Unlock()
		return false
	}
	s.adv.running = node
	s.adv.mu.Unlock()
	go func() {
		defer func() {
			s.adv.mu.Lock()
			s.adv.running = ""
			s.adv.mu.Unlock()
		}()
		state := s.log.Snapshot()
		if state == nil {
			return
		}
		set := s.advisorSettings(state, node)
		// It reads as the advisor's account, held to its folder (0212).
		reads := s.adv.reads(asHeld(s.Handler(), advisor.Account, holdOf(state, node)))
		run, err := advisor.Review(ctx, set.Settings, reads, s.conds, why, question, time.Now())
		if err != nil {
			slog.Warn("advisor: the review could not be kept", "folder", node, "err", err)
			return
		}
		slog.Info("advisor: review", "folder", node, "run", run.ID, "outcome", run.Outcome, "requests", run.Requests, "cost", run.Cost, "error", run.Error)
	}()
	return true
}

// reviewDue says whether the schedule asks for a review now: the advisor
// on, a key set, and none begun in the hours between reviews.
func reviewDue(set advisorSettings, last *conditions.AdviceRun, now time.Time) bool {
	if !set.Enabled || !set.KeySet {
		return false
	}
	return last == nil || now.Sub(last.Started) >= time.Duration(set.Every)*time.Hour
}

// Advise runs a review when one is due, a folder's at a time, looking every
// interval until ctx ends (0205, 0212). A review left running by a manager
// that stopped is ended, and those kept before advisors were a folder's are
// the Org's.
func (s *Server) Advise(ctx context.Context, every time.Duration) {
	if s.adv.reads == nil || s.conds == nil {
		return
	}
	if err := s.conds.EndRunningAdvice("the manager stopped during the review"); err != nil {
		slog.Warn("advisor: cannot end reviews left running", "err", err)
	}
	if state := s.log.Snapshot(); state != nil {
		if err := s.conds.AdoptAdvice(string(state.Org.Locations.Root())); err != nil {
			slog.Warn("advisor: cannot give the Org its reviews from before", "err", err)
		}
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if state := s.log.Snapshot(); state != nil {
			for _, node := range advisorScopes(state) {
				runs, err := s.conds.AdviceRuns(string(node), 1)
				var last *conditions.AdviceRun
				if err == nil && len(runs) > 0 {
					last = &runs[0]
				}
				if err == nil && reviewDue(s.advisorSettings(state, node), last, time.Now()) {
					s.review(ctx, node, "schedule", "")
					break // one at a time: the next, at a later look
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// advisorSaid is one folder's advisor, as the page lists it (0212).
type advisorSaid struct {
	Scope   hierarchy.NodeID `json:"scope"`
	Name    string           `json:"name"`
	Enabled bool             `json:"enabled"`
	Model   string           `json:"model"`
	Every   int              `json:"every"`
	Web     bool             `json:"web"`
	Budget  float64          `json:"budget"`
	Running bool             `json:"running"`
	Open    int              `json:"open"`
	// Whether the caller may ask it for a review, and open or dismiss
	// what it found.
	MayReview  bool `json:"may_review"`
	MayDismiss bool `json:"may_dismiss"`
}

// advice is what the folders' advisors found, and their reviews: GET
// /v1/advisor, ?scope= a folder whose advisor to show, the first the
// caller may view unless set (0212). Each folder's is shown to whoever may
// view that folder.
func (s *Server) advice(w http.ResponseWriter, r *http.Request, c call) error {
	s.adv.mu.Lock()
	running := s.adv.running
	s.adv.mu.Unlock()
	_, hasAccount := c.state.Access.Account(advisor.Account)
	t := c.state.Org.Locations
	scopes := viewableScopes(c)
	out := map[string]any{
		"available": s.adv.reads != nil, "account": advisor.Account, "account_set": hasAccount,
		// Where one may set an advisor up: an admin's of the folder.
		"may_set_up": mayAnywhere(c, access.Administer),
		"scopes":     []advisorSaid{}, "findings": []conditions.Finding{}, "runs": []conditions.AdviceRun{},
	}
	if len(scopes) == 0 {
		writeJSON(w, http.StatusOK, out)
		return nil
	}
	pick := scopes[0]
	if q := hierarchy.NodeID(r.URL.Query().Get("scope")); q != "" {
		found := false
		for _, s := range scopes {
			if s == q {
				pick, found = q, true
			}
		}
		if !found {
			return errNotFound
		}
	}
	list := make([]advisorSaid, 0, len(scopes))
	for _, node := range scopes {
		set := s.advisorSettings(c.state, node)
		open, err := s.conds.Findings(string(node), 500)
		if err != nil {
			return err
		}
		n := 0
		for _, f := range open {
			if f.State == "open" {
				n++
			}
		}
		list = append(list, advisorSaid{
			Scope: node, Name: set.ScopeName, Enabled: set.Enabled, Model: set.Model, Every: set.Every, Web: set.Web, Budget: set.Budget,
			Running: running == node, Open: n,
			MayReview:  may(c, change.Locations, t, node, access.Administer),
			MayDismiss: may(c, change.Locations, t, node, access.Configure),
		})
	}
	findings, err := s.conds.Findings(string(pick), 500)
	if err != nil {
		return err
	}
	runs, err := s.conds.AdviceRuns(string(pick), 20)
	if err != nil {
		return err
	}
	out["scope"], out["scopes"], out["findings"], out["runs"] = pick, list, findings, runs
	out["running"] = running != ""
	writeJSON(w, http.StatusOK, out)
	return nil
}

// mayAnywhere says whether the caller has right x at some Locations node.
func mayAnywhere(c call, x access.Right) bool {
	t := c.state.Org.Locations
	root := t.Root()
	for _, n := range append([]hierarchy.NodeID{root}, t.Descendants(root)...) {
		if may(c, change.Locations, t, n, x) {
			return true
		}
	}
	return false
}

// adviseNow asks a folder's advisor for a review now: POST
// /v1/advisor/run {scope, question}, scope unset where there is one advisor
// alone. With a question, the review answers it (0215). It costs that
// folder's key's money, so it is an admin's of the folder (0205, 0212).
func (s *Server) adviseNow(w http.ResponseWriter, r *http.Request, c call) error {
	var b struct {
		Scope    hierarchy.NodeID `json:"scope"`
		Question *string          `json:"question"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	question := ""
	if b.Question != nil {
		question = strings.TrimSpace(*b.Question)
		if question == "" || len(question) > advisor.QuestionMax {
			return badRequest("question: 1 to 2000 characters")
		}
	}
	scopes := advisorScopes(c.state)
	node := b.Scope
	if node == "" && len(scopes) == 1 {
		node = scopes[0]
	}
	t := c.state.Org.Locations
	if node == "" {
		if !mayAnywhere(c, access.Administer) {
			return errNotFound
		}
		return &apiError{http.StatusConflict, "no folder has an advisor: set system.advisor.key at a folder first"}
	}
	if !may(c, change.Locations, t, node, access.Administer) {
		return errNotFound
	}
	if s.adv.reads == nil {
		return &apiError{http.StatusConflict, "this manager has no advisor"}
	}
	isScope := false
	for _, s := range scopes {
		isScope = isScope || s == node
	}
	if !isScope {
		return &apiError{http.StatusConflict, "no Anthropic API key is set at this folder: set system.advisor.key here first"}
	}
	if !s.review(context.Background(), node, "asked by "+string(c.actor), question) {
		return &apiError{http.StatusConflict, "a review is under way: try again when it is done"}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "scope": node, "question": question != ""})
	return nil
}

// adviceState opens, dismisses or resolves a finding: POST
// /v1/advisor/findings/{id} {state, why}, why for resolved alone, and
// optional: a person who fixed what it found says so (0215). An operator's
// of the folder whose advisor found it (0212).
func (s *Server) adviceState(w http.ResponseWriter, r *http.Request, c call) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return badRequest("a finding is named by its number")
	}
	scope, err := s.conds.FindingScope(id)
	if errors.Is(err, conditions.ErrNoFinding) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !may(c, change.Locations, t, hierarchy.NodeID(scope), access.Configure) {
		return errNotFound
	}
	var b struct {
		State string `json:"state"`
		Why   string `json:"why"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if b.State != "open" && b.State != "dismissed" && b.State != "resolved" {
		return badRequest("state is open, dismissed or resolved")
	}
	why := strings.TrimSpace(b.Why)
	if why != "" && b.State != "resolved" {
		return badRequest("why is for resolved alone")
	}
	if len(why) > 2000 {
		return badRequest("why: at most 2000 characters")
	}
	if err := s.conds.SetFindingState(id, b.State, string(c.actor), why); err != nil {
		if errors.Is(err, conditions.ErrNoFinding) {
			return errNotFound
		}
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "state": b.State})
	return nil
}
