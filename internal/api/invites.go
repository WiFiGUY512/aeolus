package api

// Invites (0336): an owner makes a link for someone to join by, with the
// roles it gives; they open it, choose their account and their own
// password, and are signed in. The link carries the invite's secret, and
// only it: the manager answers it once, when the invite is made, and keeps
// its hash.

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/sessions"
)

const (
	// InviteDays is how long an invite is good unless it says, and
	// InviteDaysMax the longest it may be.
	InviteDays    = 7
	InviteDaysMax = 30
	// RegisterPath is where an invite's link opens, before its code.
	RegisterPath = "/#/register/"
)

// accountPattern is an account one registers: 2 to 32 characters, lower
// case letters, digits, dots, underscores and hyphens, from a letter.
var accountPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,31}$`)

// roleSaid is one of the roles an invite gives, with its folder's name.
type roleSaid struct {
	Tree  change.TreeName `json:"tree"`
	Node  string          `json:"node"`
	Role  string          `json:"role"`
	Where string          `json:"where"`
}

// inviteSaid is an invite as an owner sees it: never its secret or hash.
type inviteSaid struct {
	ID      string           `json:"id"`
	State   string           `json:"state"` // open, used, revoked or expired
	Roles   []roleSaid       `json:"roles"`
	Owner   bool             `json:"owner"`
	By      access.AccountID `json:"by"`
	Made    *time.Time       `json:"made,omitempty"`
	Expires time.Time        `json:"expires"`
	Note    string           `json:"note,omitempty"`
	UsedBy  access.AccountID `json:"used_by,omitempty"`
	UsedAt  *time.Time       `json:"used_at,omitempty"`
}

// rolesSaid is an invite's roles, each with its folder's name, or its ID
// where the folder is gone.
func rolesSaid(st *change.State, iv access.Invite) []roleSaid {
	out := make([]roleSaid, 0, len(iv.Grants))
	for _, g := range iv.Grants {
		where := string(g.Node)
		if _, t, err := treeOf(st, g.Tree); err == nil {
			if n, ok := t.Node(g.Node); ok {
				where = n.Name
			}
		}
		out = append(out, roleSaid{Tree: change.TreeName(g.Tree), Node: string(g.Node), Role: g.Role.String(), Where: where})
	}
	return out
}

func inviteView(st *change.State, iv access.Invite, now time.Time) inviteSaid {
	return inviteSaid{ID: iv.ID, State: iv.State(now), Roles: rolesSaid(st, iv), Owner: iv.Owner, By: iv.By, Expires: iv.Expires, Note: iv.Note, UsedBy: iv.UsedBy}
}

// ownerOnly refuses a caller who is not an owner.
func ownerOnly(c call) error {
	if !owns(c) {
		return &apiError{http.StatusForbidden, "invites are an owner's (0336)"}
	}
	return nil
}

// inviteList is every invite, newest first, with when each was made and
// used, by the change log: GET /v1/invites. An owner's.
func (s *Server) inviteList(w http.ResponseWriter, _ *http.Request, c call) error {
	if err := ownerOnly(c); err != nil {
		return err
	}
	made := map[string]time.Time{}
	used := map[string]time.Time{}
	for kind, at := range map[change.Kind]map[string]time.Time{change.AddInvite: made, change.UseInvite: used} {
		list, err := s.log.OfKind(kind)
		if err != nil {
			return err
		}
		for _, e := range list {
			at[e.Op.TokenID] = e.At
		}
	}
	now := time.Now()
	out := []inviteSaid{}
	for _, iv := range c.state.Access.Invites() {
		v := inviteView(c.state, iv, now)
		if at, ok := made[iv.ID]; ok {
			v.Made = &at
		}
		if at, ok := used[iv.ID]; ok {
			v.UsedAt = &at
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out, "days": InviteDays, "days_max": InviteDaysMax})
	return nil
}

// inviteAdd makes an invite and answers its link, once: POST /v1/invites
// {roles: [{tree, node, role}], owner?, days?, note?, reason?}. An owner's.
func (s *Server) inviteAdd(w http.ResponseWriter, r *http.Request, c call) error {
	if err := ownerOnly(c); err != nil {
		return err
	}
	var b struct {
		Roles  []change.RoleOn `json:"roles"`
		Owner  bool            `json:"owner"`
		Days   int             `json:"days"`
		Note   string          `json:"note"`
		Reason string          `json:"reason"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if b.Days == 0 {
		b.Days = InviteDays
	}
	if b.Days < 1 || b.Days > InviteDaysMax {
		return badRequest("an invite is good for 1 to %d days", InviteDaysMax)
	}
	code, id, hash, err := access.NewInvite()
	if err != nil {
		return err
	}
	op := change.Op{Kind: change.AddInvite, Account: c.actor, TokenID: id, TokenHash: hash, Roles: b.Roles, Note: strings.TrimSpace(b.Note),
		Expires: time.Now().UTC().Add(time.Duration(b.Days) * 24 * time.Hour).Truncate(time.Second)}
	if b.Owner {
		op.Owner = &b.Owner
	}
	e, err := s.log.Commit(string(c.actor), b.Reason, op)
	if err != nil {
		return err
	}
	st := s.log.Snapshot()
	iv, _ := st.Access.Invite(id)
	v := inviteView(st, iv, time.Now())
	v.Made = &e.At
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	writeJSON(w, http.StatusOK, map[string]any{"invite": v, "code": code, "path": RegisterPath + code, "link": scheme + "://" + r.Host + RegisterPath + code})
	return nil
}

// inviteRevoke stops an invite before it is used: DELETE
// /v1/invites/{id}. An owner's.
func (s *Server) inviteRevoke(w http.ResponseWriter, r *http.Request, c call) error {
	if err := ownerOnly(c); err != nil {
		return err
	}
	if _, err := s.log.Commit(string(c.actor), "", change.Op{Kind: change.RevokeInvite, TokenID: r.PathValue("id")}); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

// checkInvite is the commit check's part for invites (0336), by the clock,
// so not Apply's, which the change log replays: an invite is used only
// while it is good, and made good from now for at most InviteDaysMax days.
func checkInvite(state *change.State, op change.Op, now time.Time) error {
	switch op.Kind {
	case change.UseInvite:
		return state.Access.UsableInvite(op.TokenID, now)
	case change.AddInvite:
		if !op.Expires.After(now) || op.Expires.After(now.Add(InviteDaysMax*24*time.Hour+time.Hour)) {
			return badRequest("an invite is good from now for 1 to %d days", InviteDaysMax)
		}
	}
	return nil
}

// invited finds the invite a code is for, and that it is still good,
// answering the caller itself where it is not: a code the manager never
// made counts as a failed try from the caller's address, as a wrong
// password does. ok is false once it has answered.
func (s *Server) invited(w http.ResponseWriter, r *http.Request, code string) (st *change.State, iv access.Invite, ok bool) {
	if s.sessions == nil {
		writeError(w, http.StatusNotFound, "this manager signs in with tokens only")
		return nil, iv, false
	}
	addr := "address:" + source(r)
	if d := s.limiter.Locked(addr); d > 0 {
		tooMany(w, d)
		return nil, iv, false
	}
	st = s.log.Snapshot()
	if st == nil {
		writeError(w, http.StatusNotFound, access.ErrBadInvite.Error())
		return nil, iv, false
	}
	iv, err := st.Access.FindInvite(code)
	if err != nil {
		s.limiter.Fail(addr)
		writeError(w, http.StatusNotFound, err.Error())
		return nil, iv, false
	}
	if err := st.Access.UsableInvite(iv.ID, time.Now()); err != nil {
		writeError(w, http.StatusGone, err.Error()+": ask whoever sent it for a new one")
		return nil, iv, false
	}
	return st, iv, true
}

// registerRead says what an invite gives, for the one it is for: GET
// /v1/register/{code}. It needs no sign-in: the code is the credential.
func (s *Server) registerRead(w http.ResponseWriter, r *http.Request) {
	st, iv, ok := s.invited(w, r, r.PathValue("code"))
	if !ok {
		return
	}
	by := string(iv.By)
	if acc, ok := st.Access.Account(iv.By); ok && acc.Name != "" {
		by = acc.Name
	}
	org := ""
	if n, ok := st.Org.Locations.Node(st.Org.Locations.Root()); ok {
		org = n.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{"org": org, "by": by, "roles": rolesSaid(st, iv), "owner": iv.Owner, "expires": iv.Expires, "note": iv.Note,
		"password_min": access.PasswordMin})
}

// register makes the account an invite is for, with the password its owner
// chose, and signs them in: POST /v1/register {code, account, name?,
// password, keep?}. The manager makes it in its own name, the invite's
// secret checked: no one else may.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Code     string `json:"code"`
		Account  string `json:"account"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Keep     bool   `json:"keep"`
	}
	if err := readJSON(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, iv, ok := s.invited(w, r, b.Code)
	if !ok {
		return
	}
	id := access.AccountID(strings.ToLower(strings.TrimSpace(b.Account)))
	if !accountPattern.MatchString(string(id)) {
		writeError(w, http.StatusBadRequest, "an account is 2 to 32 characters: lower case letters, digits, dots, underscores or hyphens, starting with a letter")
		return
	}
	// Names from the manager's own, as its advisor's (0205), are kept for
	// it, whether or not the account is there yet.
	if _, taken := st.Access.Account(id); taken || strings.HasPrefix(string(id), change.SystemActor) {
		writeError(w, http.StatusConflict, fmt.Sprintf("the account %s is taken: choose another", id))
		return
	}
	name := strings.TrimSpace(b.Name)
	if utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		writeError(w, http.StatusBadRequest, "a name is at most 64 characters, none of them control characters")
		return
	}
	if err := access.CheckPasswordRules(id, b.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := access.HashPassword(b.Password)
	if err != nil {
		s.fail(w, r, string(id), err)
		return
	}
	op := change.Op{Kind: change.UseInvite, TokenID: iv.ID, Account: id, Name: name, PasswordHash: hash}
	if _, err := s.log.Commit(change.SystemActor, fmt.Sprintf("%s registered with an invite from %s (0336)", id, iv.By), op); err != nil {
		s.fail(w, r, string(id), err)
		return
	}
	src := source(r)
	s.limiter.Succeed("address:" + src)
	if err := s.startSession(w, r, string(id), hash, b.Keep); err != nil {
		s.fail(w, r, string(id), err)
		return
	}
	s.sessions.Record(sessions.SignIn{Account: string(id), Source: src, OK: true, Why: sessions.Registered})
	acc, _ := s.log.Snapshot().Access.Account(id)
	writeJSON(w, http.StatusOK, map[string]any{"account": acc.ID, "name": acc.Name, "owner": acc.Owner, "must_change": false})
}
