package api

// The admin section (0176): the Org's people, each account's roles, tokens
// and sessions, and the sign-in record. Who sees whom follows 0177: an
// owner, the super admin, sees everyone; an admin or a manager, the
// accounts holding roles where it may give them; anyone, itself.

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/sessions"
)

// grantSaid is one of an account's roles, and whether the caller may take
// it away.
type grantSaid struct {
	Tree    string           `json:"tree"`
	Node    hierarchy.NodeID `json:"node"`
	Role    string           `json:"role"`
	MayTake bool             `json:"may_take"`
}

// accountSaid is an account as the Admin page shows it.
type accountSaid struct {
	ID          access.AccountID `json:"account"`
	Name        string           `json:"name"`
	Owner       bool             `json:"owner"`
	State       string           `json:"state"` // active, disabled or removed
	HasPassword bool             `json:"has_password"`
	MustChange  bool             `json:"must_change"`
	LastSignIn  *time.Time       `json:"last_sign_in,omitempty"`
	Grants      []grantSaid      `json:"grants"`
}

// tokenSaid is one of an account's live tokens: when it was issued, by
// whom and why, and when it was last used, from where. Never its secret.
type tokenSaid struct {
	ID       string     `json:"id"`
	IssuedAt *time.Time `json:"issued_at,omitempty"`
	IssuedBy string     `json:"issued_by,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	LastUsed *time.Time `json:"last_used,omitempty"`
	Source   string     `json:"source,omitempty"`
}

// sessionSaid is one of an account's sessions, by the first characters of
// its ID: enough to end it, too few to stand for it anywhere else.
type sessionSaid struct {
	ID       string    `json:"id"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
	Expires  time.Time `json:"expires"`
	Keep     bool      `json:"keep"`
	Source   string    `json:"source"`
	Agent    string    `json:"agent"`
	Current  bool      `json:"current"`
}

const sessionShown = 16

// owns says whether the caller is an owner, the super admin.
func owns(c call) bool {
	acc, _ := c.state.Access.Account(c.actor)
	return acc.Owner && acc.Active()
}

// grantChain is a grant's folder's ancestry, nil where it is gone.
func grantChain(st *change.State, g access.Grant) []hierarchy.NodeID {
	_, t, err := treeOf(st, g.Tree)
	if err != nil {
		return nil
	}
	return t.Ancestry(g.Node)
}

// sees says whether the caller may see an account on the Admin page.
func sees(c call, id access.AccountID) bool {
	if id == c.actor || owns(c) {
		return true
	}
	for _, g := range c.state.Access.Grants() {
		if g.Account != id {
			continue
		}
		if chain := grantChain(c.state, g); chain != nil && len(c.state.Access.GivesAt(c.actor, g.Tree, chain)) > 0 {
			return true
		}
	}
	return false
}

// seesSignIns says whether the caller may read an account's sign-ins: its
// own; anyone's, as an owner; or of one holding a role where the caller
// holds the sign-ins right (0177).
func seesSignIns(c call, id string) bool {
	if id == string(c.actor) || owns(c) {
		return true
	}
	for _, g := range c.state.Access.Grants() {
		if string(g.Account) != id {
			continue
		}
		if chain := grantChain(c.state, g); chain != nil && c.state.Access.May(c.actor, g.Tree, chain, access.SignIns) {
			return true
		}
	}
	return false
}

// accountView is an account with the grants the caller may see: on
// folders it may view.
func accountView(c call, acc access.Account, last map[string]time.Time) accountSaid {
	v := accountSaid{ID: acc.ID, Name: acc.Name, Owner: acc.Owner, State: "active", HasPassword: acc.Password != "", MustChange: acc.MustChange, Grants: []grantSaid{}}
	switch {
	case acc.Removed:
		v.State = "removed"
	case acc.Disabled:
		v.State = "disabled"
	}
	if at, ok := last[string(acc.ID)]; ok {
		v.LastSignIn = &at
	}
	for _, g := range c.state.Access.Grants() {
		if g.Account != acc.ID {
			continue
		}
		chain := grantChain(c.state, g)
		if chain == nil || !c.state.Access.May(c.actor, g.Tree, chain, access.View) {
			continue
		}
		v.Grants = append(v.Grants, grantSaid{Tree: g.Tree, Node: g.Node, Role: g.Role.String(), MayTake: c.state.Access.MayGive(c.actor, g.Tree, chain, g.Role)})
	}
	return v
}

func (s *Server) lastSignIns() map[string]time.Time {
	if s.sessions == nil {
		return nil
	}
	last, _ := s.sessions.LastSignIns()
	return last
}

// accountList is the people the caller may see: GET /v1/accounts.
func (s *Server) accountList(w http.ResponseWriter, _ *http.Request, c call) error {
	last := s.lastSignIns()
	out := []accountSaid{}
	for _, acc := range c.state.Access.Accounts() {
		if sees(c, acc.ID) {
			out = append(out, accountView(c, acc, last))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out, "may_add": owns(c)})
	return nil
}

// accountRead is one account: GET /v1/accounts/{id}. Its tokens and
// sessions are its own and an owner's to see; its sign-ins, as
// seesSignIns says.
func (s *Server) accountRead(w http.ResponseWriter, r *http.Request, c call) error {
	id := access.AccountID(r.PathValue("id"))
	acc, ok := c.state.Access.Account(id)
	if !ok || !sees(c, id) {
		return errNotFound
	}
	res := map[string]any{"account": accountView(c, acc, s.lastSignIns()), "may_run": owns(c) && id != c.actor}
	if id == c.actor || owns(c) {
		toks, err := s.tokensOf(c, id)
		if err != nil {
			return err
		}
		res["tokens"] = toks
		if s.sessions != nil {
			list, err := s.sessions.Of(string(id))
			if err != nil {
				return err
			}
			out := []sessionSaid{}
			for _, ses := range list {
				out = append(out, sessionSaid{ID: ses.ID[:sessionShown], Created: ses.Created, LastUsed: ses.LastUsed, Expires: ses.Expires(), Keep: ses.Keep,
					Source: ses.Source, Agent: ses.Agent, Current: c.session != nil && c.session.ID == ses.ID})
			}
			res["sessions"] = out
		}
	}
	if s.sessions != nil && seesSignIns(c, string(id)) {
		list, err := s.sessions.SignIns(string(id), 20)
		if err != nil {
			return err
		}
		res["signins"] = list
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// tokensOf is an account's live tokens, with who issued each and when, by
// the change log, and when each was last used.
func (s *Server) tokensOf(c call, id access.AccountID) ([]tokenSaid, error) {
	issued, err := s.log.OfKind(change.IssueToken)
	if err != nil {
		return nil, err
	}
	out := []tokenSaid{}
	var ids []string
	for _, tok := range c.state.Access.TokensOf(id) {
		if tok.Revoked {
			continue
		}
		t := tokenSaid{ID: tok.ID}
		for _, e := range issued {
			if e.Op.TokenID == tok.ID {
				at := e.At
				t.IssuedAt, t.IssuedBy, t.Reason = &at, e.Actor, e.Reason
			}
		}
		out = append(out, t)
		ids = append(ids, tok.ID)
	}
	if s.sessions != nil {
		uses, err := s.sessions.TokenUses(ids)
		if err != nil {
			return nil, err
		}
		for i, t := range out {
			if u, ok := uses[t.ID]; ok {
				at := u.At
				out[i].LastUsed, out[i].Source = &at, u.Source
			}
		}
	}
	// Newest first.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].IssuedAt, out[j].IssuedAt
		return a != nil && (b == nil || a.After(*b))
	})
	return out, nil
}

// accountAdd adds an account with a temporary password, which it answers
// once: POST /v1/accounts {account, name, owner?, reason?}. An owner's to
// do (0176).
func (s *Server) accountAdd(w http.ResponseWriter, r *http.Request, c call) error {
	var b struct {
		Account access.AccountID `json:"account"`
		Name    string           `json:"name"`
		Owner   bool             `json:"owner"`
		Reason  string           `json:"reason"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	b.Account = access.AccountID(strings.ToLower(strings.TrimSpace(string(b.Account))))
	if _, err := s.log.Commit(string(c.actor), b.Reason, change.Op{Kind: change.AddAccount, Account: b.Account, Name: strings.TrimSpace(b.Name)}); err != nil {
		return err
	}
	if b.Owner {
		if _, err := s.log.Commit(string(c.actor), b.Reason, change.Op{Kind: change.SetOwner, Account: b.Account, Owner: &b.Owner}); err != nil {
			return err
		}
	}
	plain, err := s.temporary(c, b.Account, "a new account's first password")
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": b.Account, "password": plain})
	return nil
}

// temporary gives an account a temporary password, to be changed at its
// next sign-in, and answers it.
func (s *Server) temporary(c call, id access.AccountID, reason string) (string, error) {
	plain, err := access.TemporaryPassword()
	if err != nil {
		return "", err
	}
	hash, err := access.HashPassword(plain)
	if err != nil {
		return "", err
	}
	if _, err := s.log.Commit(string(c.actor), reason, change.Op{Kind: change.SetPassword, Account: id, PasswordHash: hash, MustChange: true}); err != nil {
		return "", err
	}
	return plain, nil
}

// accountReset resets another account's password to a temporary one,
// which it answers once, and ends its sessions: POST
// /v1/accounts/{id}/password. An owner's to do (0176); one's own is
// changed at /v1/session/password.
func (s *Server) accountReset(w http.ResponseWriter, r *http.Request, c call) error {
	id := access.AccountID(r.PathValue("id"))
	if _, ok := c.state.Access.Account(id); !ok || !sees(c, id) {
		return errNotFound
	}
	if id == c.actor {
		return badRequest("change your own password on the Password page, POST /v1/session/password")
	}
	plain, err := s.temporary(c, id, "reset by an owner")
	if err != nil {
		return err
	}
	ended := 0
	if s.sessions != nil {
		if ended, err = s.sessions.DeleteFor(string(id), ""); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": id, "password": plain, "sessions_ended": ended})
	return nil
}

// sessionsEnd ends an account's sessions: DELETE
// /v1/accounts/{id}/sessions, every one but the caller's own; or DELETE
// /v1/accounts/{id}/sessions/{session}, the one whose ID starts so. Its
// own, or an owner's to do.
func (s *Server) sessionsEnd(w http.ResponseWriter, r *http.Request, c call) error {
	id := access.AccountID(r.PathValue("id"))
	if _, ok := c.state.Access.Account(id); !ok || !sees(c, id) {
		return errNotFound
	}
	if id != c.actor && !owns(c) {
		return &apiError{http.StatusForbidden, "ending another's sessions needs an owner (0176)"}
	}
	if s.sessions == nil {
		return errNotFound
	}
	keep := ""
	if c.session != nil {
		keep = c.session.ID
	}
	one := r.PathValue("session")
	if one == "" {
		ended, err := s.sessions.DeleteFor(string(id), keep)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]any{"ended": ended})
		return nil
	}
	list, err := s.sessions.Of(string(id))
	if err != nil {
		return err
	}
	for _, ses := range list {
		if len(one) == sessionShown && strings.HasPrefix(ses.ID, one) {
			if err := s.sessions.Delete(ses.ID); err != nil {
				return err
			}
			writeJSON(w, http.StatusOK, map[string]any{"ended": 1})
			return nil
		}
	}
	return errNotFound
}

// signIns is the sign-in record the caller may read, newest first: GET
// /v1/signins, ?account= for one account's, ?ok=true or false for those
// that worked or failed, ?limit= at most (200 unless set, at most 1000).
// A try under an account no one has is an owner's to see.
func (s *Server) signIns(w http.ResponseWriter, r *http.Request, c call) error {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, map[string]any{"signins": []sessions.SignIn{}})
		return nil
	}
	q := r.URL.Query()
	limit := 200
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return badRequest("limit is 1 to 1000")
		}
		limit = n
	}
	var want *bool
	if v := q.Get("ok"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return badRequest("ok is true or false")
		}
		want = &b
	}
	account := strings.ToLower(q.Get("account"))
	list, err := s.sessions.SignIns(account, 1000)
	if err != nil {
		return err
	}
	out := []sessions.SignIn{}
	seen := map[string]bool{}
	for _, si := range list {
		ok, known := seen[si.Account]
		if !known {
			ok = seesSignIns(c, si.Account)
			seen[si.Account] = ok
		}
		if !ok || (want != nil && si.OK != *want) {
			continue
		}
		out = append(out, si)
		if len(out) == limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"signins": out})
	return nil
}
