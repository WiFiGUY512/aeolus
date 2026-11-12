package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/sessions"
)

// Signing in with a password (0176): a session in a cookie, beside the
// bearer tokens scripts, the MCP adapter and the APs keep using.
const (
	// SessionCookie holds a session's secret: HttpOnly, Secure,
	// SameSite=Strict.
	SessionCookie = "aeolus_session"
	// SessionHeader must come with a change made with the cookie. A page
	// from another site can't add it, which keeps such a page from making
	// changes in a signed-in person's name.
	SessionHeader = "X-Aeolus-Session"
)

// WithVersion gives the server its own release, which whoami says: the
// page shows it, and a problem report names it.
func (s *Server) WithVersion(v string) *Server {
	s.version = v
	return s
}

// WithSessions gives the server its sessions and sign-in record, and turns
// on signing in with a password and the limits on guessing.
func (s *Server) WithSessions(store *sessions.Store) *Server {
	s.sessions, s.limiter = store, sessions.NewLimiter(nil)
	return s
}

// source is the address a request came from.
func source(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tooMany answers a locked caller, saying when to try again.
func tooMany(w http.ResponseWriter, d time.Duration) {
	secs := int(d.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeError(w, http.StatusTooManyRequests, fmt.Sprintf("too many failed tries: try again in %d s", secs))
}

// authenticate finds who a request is from: by its bearer token, else by
// its session cookie. code is the status to refuse with, or 0.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (c call, code int, msg string) {
	state := s.log.Snapshot()
	if state == nil {
		return call{}, http.StatusUnauthorized, "missing or invalid token"
	}
	// The manager's own request of itself, as one of its accounts (0205).
	if who, ok := asAccount(r); ok {
		if acc, found := state.Access.Account(who); found && acc.Active() {
			return call{actor: acc.ID, state: state, held: heldOf(r)}, 0, ""
		}
		return call{}, http.StatusUnauthorized, "the account " + string(who) + " is not there, or not active"
	}
	addr := "address:" + source(r)
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if s.limiter != nil {
			if d := s.limiter.Locked(addr); d > 0 {
				tooMany(w, d)
				return call{}, -1, ""
			}
		}
		token = strings.TrimSpace(token)
		who, err := state.Access.Authenticate(token)
		if err != nil {
			if s.limiter != nil {
				s.limiter.Fail(addr)
			}
			return call{}, http.StatusUnauthorized, "missing or invalid token"
		}
		// When it was last used, and from where, for the Admin page; its
		// ID is the part before the secret.
		if s.sessions != nil {
			s.sessions.TokenUsed(strings.SplitN(token, ".", 3)[1], source(r))
		}
		return call{actor: who, state: state}, 0, ""
	}
	if s.sessions != nil {
		if ck, err := r.Cookie(SessionCookie); err == nil {
			ses, err := s.sessions.Lookup(ck.Value)
			if err == nil {
				acc, ok := state.Access.Account(access.AccountID(ses.Account))
				// A session ends when its account is disabled or removed, or
				// its password is changed elsewhere or reset (0176).
				if ok && acc.Active() && sessions.PwTag(acc.Password) == ses.PwTag {
					return call{actor: acc.ID, state: state, session: &ses}, 0, ""
				}
				s.sessions.Delete(ses.ID)
			}
			return call{}, http.StatusUnauthorized, "signed out: sign in again"
		}
	}
	return call{}, http.StatusUnauthorized, "missing or invalid token"
}

// mayBeforeChange says whether a request may be made with a temporary
// password, before it is changed: finding out who one is, and changing it.
func mayBeforeChange(r *http.Request) bool {
	return (r.Method == http.MethodGet && r.URL.Path == "/v1/whoami") ||
		(r.Method == http.MethodPost && r.URL.Path == "/v1/session/password")
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

type signInBody struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	Keep     bool   `json:"keep"`
}

// signIn starts a session (POST /v1/session): an account and its password.
// A wrong account and a wrong password get the same answer, in the same
// time; every try is recorded, and too many failures lock the account and
// the address for a while.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeError(w, http.StatusNotFound, "this manager signs in with tokens only")
		return
	}
	var b signInBody
	if err := readJSON(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := strings.ToLower(strings.TrimSpace(b.Account))
	if len(id) > 64 || strings.IndexFunc(id, unicode.IsControl) >= 0 {
		id = "(not an account)"
	}
	src := source(r)
	keys := []string{"account:" + id, "address:" + src}
	record := func(ok bool, why string) {
		s.sessions.Record(sessions.SignIn{Account: id, Source: src, OK: ok, Why: why})
	}
	if d := s.limiter.Locked(keys...); d > 0 {
		record(false, sessions.Locked)
		tooMany(w, d)
		return
	}
	state := s.log.Snapshot()
	var acc access.Account
	found := false
	if state != nil {
		acc, found = state.Access.Account(access.AccountID(id))
	}
	// Checked against a dummy where there is no account or no password,
	// which takes as long.
	good := access.VerifyPassword(acc.Password, b.Password)
	why := sessions.OK
	switch {
	case !found || !good:
		why = sessions.Wrong
		if found && acc.Password == "" {
			why = sessions.NoPassword
		}
	case !acc.Active():
		why = sessions.Inactive
	}
	if why != sessions.OK {
		s.limiter.Fail(keys...)
		record(false, why)
		writeError(w, http.StatusUnauthorized, "wrong account or password")
		return
	}
	s.limiter.Succeed(keys...)
	if err := s.startSession(w, r, id, acc.Password, b.Keep); err != nil {
		s.fail(w, r, id, err)
		return
	}
	if acc.MustChange {
		record(true, sessions.MustChanged)
	} else {
		record(true, sessions.OK)
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acc.ID, "name": acc.Name, "owner": acc.Owner, "must_change": acc.MustChange})
}

// startSession starts a session for an account with the password hash it
// has, and gives the browser its cookie: for the browser's own session,
// unless keep, which keeps it for KeepTimeout.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, id, pwHash string, keep bool) error {
	secret, _, err := s.sessions.Create(id, sessions.PwTag(pwHash), keep, source(r), r.UserAgent())
	if err != nil {
		return err
	}
	ck := &http.Cookie{Name: SessionCookie, Value: secret, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
	if keep {
		ck.MaxAge = int(sessions.KeepTimeout / time.Second)
	}
	http.SetCookie(w, ck)
	return nil
}

// signOut ends the caller's session (DELETE /v1/session) and clears its
// cookie. With none, it does nothing, and says so the same.
func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	if s.sessions != nil {
		if ck, err := r.Cookie(SessionCookie); err == nil {
			if ses, err := s.sessions.Lookup(ck.Value); err == nil {
				s.sessions.Delete(ses.ID)
			}
		}
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type passwordBody struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// changePassword sets the caller's own password (POST /v1/session/password),
// with the current one, unless the account has none yet. Its other sessions
// end; the one that changed it goes on.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, c call) error {
	var b passwordBody
	if err := readJSON(r, &b); err != nil {
		return err
	}
	acc, _ := c.state.Access.Account(c.actor)
	key := "account:" + string(c.actor)
	if acc.Password != "" {
		if s.limiter != nil {
			if d := s.limiter.Locked(key); d > 0 {
				tooMany(w, d)
				return nil
			}
		}
		if !access.VerifyPassword(acc.Password, b.Current) {
			if s.limiter != nil {
				s.limiter.Fail(key)
			}
			return &apiError{http.StatusForbidden, "the current password is wrong"}
		}
		if s.limiter != nil {
			s.limiter.Succeed(key)
		}
	}
	if err := access.CheckPasswordRules(c.actor, b.New); err != nil {
		return badRequest("%v", err)
	}
	if acc.MustChange && b.New == b.Current {
		return badRequest("choose a password of your own, not the temporary one")
	}
	hash, err := access.HashPassword(b.New)
	if err != nil {
		return err
	}
	// This session takes the new password before it is in force, so a
	// request on it meanwhile isn't taken for one signed in with the old;
	// if the change fails, it goes back.
	keep := ""
	if s.sessions != nil && c.session != nil {
		keep = c.session.ID
		if err := s.sessions.Retag(keep, sessions.PwTag(hash)); err != nil {
			return err
		}
	}
	if _, err := s.log.Commit(string(c.actor), "changed their own password", change.Op{Kind: change.SetPassword, Account: c.actor, PasswordHash: hash}); err != nil {
		if keep != "" {
			s.sessions.Retag(keep, c.session.PwTag)
		}
		return err
	}
	ended := 0
	if s.sessions != nil {
		if ended, err = s.sessions.DeleteFor(string(c.actor), keep); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "other_sessions_ended": ended})
	return nil
}
