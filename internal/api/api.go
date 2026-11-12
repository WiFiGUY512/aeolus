// Package api is the manager's one door (0003, 0026): JSON over HTTPS. The
// UI, the CLI and the MCP adapter all use it; none has a private path into
// the database.
//
// Every request except /healthz and /v1/enroll carries "Authorization:
// Bearer <token>": an account's token, or on the /v1/ap/ routes an AP's
// (0038).
// Every write is one change, logged with who made it and when, and with a
// note if the caller gives one (0062); it is authorized and checked at commit
// time (0029, 0030). Secrets are never returned in plain text (0027), and a
// node the caller cannot view answers 404, not 403, so the API does not
// reveal what exists.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/bundle"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/changelog"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/dhcpwatch"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/keys"
	"github.com/WiFiGUY512/aeolus/internal/library"
	"github.com/WiFiGUY512/aeolus/internal/schema"
	"github.com/WiFiGUY512/aeolus/internal/secret"
	"github.com/WiFiGUY512/aeolus/internal/sessions"
)

// Server serves the API.
type Server struct {
	log       *changelog.Log
	schema    *schema.Schema
	box       *secret.Box
	conds     *conditions.Store
	started   time.Time       // when this manager began: what was done before it was counted by none (0141)
	watch     *dhcpwatch.Book // what the manager's DHCP listeners hear (0068); nil if none
	psks      pskCache        // per-user keys' PSKs, worked out (0070)
	agents    *bundle.Store   // the agent bundles it offers its APs (0079); nil if none
	agent     bundle.Bundle   // its own release's bundle
	updates   string          // where the updater takes requests and says what came of them (0183); "" if none
	files     []string        // the manager's database files, whose sizes its health says (0219)
	version   string          // its own release, as whoami says it; "" unless told
	install   *Install        // what it hands out to an AP installing from it (0083); nil if nothing
	arrive    arrivals        // the times given to attempts from APs whose clocks are unset (0118)
	said      factsSaid       // when each AP last changed what it says it is (0122)
	rounds    rounds          // the rounds of tests asked by hand that are being worked at (0129)
	traces    traceBook       // the clients traced, and what the APs say of them (0144)
	live      liveHub         // the lines the APs hold open to the manager (0145)
	watching  watchHub        // who watches which APs live, and their last 5-second points (0207)
	active    activity        // the AP each client was last active on (0185)
	pcapDir   string          // where the captures people start are kept (0148); none are taken where it is empty
	terms     terms           // the terminals open on APs (0198)
	adv       advising        // the advisor (0205)
	ft        ftBook          // each AP's 802.11r key holders, as it last said them (0224)
	trust     trustFleet      // what the fleet calls its own, for airscan to trust (0304)
	reports   reportBook      // what the manager could not take of each AP's state reports lately (0290)
	read      testsRead       // the client tests over, as they were read (0285)
	pcapMu    sync.Mutex      // one at a time at a capture's files and what is noted beside them
	suggestTo string          // where suggestions are sent on (0323); "" keeps them here

	sessions *sessions.Store   // sign-in sessions and the record of sign-ins (0176); nil signs in with tokens only
	limiter  *sessions.Limiter // failed sign-ins and tokens, which lock for a while (0176)
}

// New returns a Server. The log should be opened with Check(sch) as its
// commit check. conds records what APs say and do (0039).
func New(log *changelog.Log, sch *schema.Schema, box *secret.Box, conds *conditions.Store) *Server {
	return &Server{log: log, schema: sch, box: box, conds: conds, started: time.Now()}
}

// Check is the commit check every change passes: permissions against the
// live state (0030), then the field schema (0027).
func Check(sch *schema.Schema) func(*change.State, string, change.Op) error {
	return func(state *change.State, actor string, op change.Op) error {
		if err := change.Authorize(state, actor, op); err != nil {
			return err
		}
		if err := change.Guard(op); err != nil {
			return err
		}
		if err := change.GuardIn(state, op); err != nil {
			return err
		}
		if err := checkTestNetwork(state, op, time.Now()); err != nil {
			return err
		}
		if err := checkInvite(state, op, time.Now()); err != nil {
			return err
		}
		return sch.CheckOp(op)
	}
}

// Handler returns the API's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", root)
	mux.HandleFunc("GET /healthz", s.health)
	// Signing in with a password, and out, and changing one's own (0176).
	mux.HandleFunc("POST /v1/session", s.signIn)
	mux.HandleFunc("DELETE /v1/session", s.signOut)
	mux.Handle("POST /v1/session/password", s.auth(s.changePassword))
	mux.Handle("GET /v1/whoami", s.auth(s.whoami))
	// What people ask Aeolus to do, from any page (0323).
	mux.Handle("POST /v1/suggestions", s.auth(s.addSuggestion))
	mux.Handle("GET /v1/suggestions", s.auth(s.suggestions))
	// The admin section (0176).
	mux.Handle("GET /v1/accounts", s.auth(s.accountList))
	mux.Handle("POST /v1/accounts", s.auth(s.accountAdd))
	mux.Handle("GET /v1/accounts/{id}", s.auth(s.accountRead))
	mux.Handle("POST /v1/accounts/{id}/password", s.auth(s.accountReset))
	mux.Handle("DELETE /v1/accounts/{id}/sessions", s.auth(s.sessionsEnd))
	mux.Handle("DELETE /v1/accounts/{id}/sessions/{session}", s.auth(s.sessionsEnd))
	mux.Handle("GET /v1/signins", s.auth(s.signIns))
	// Invites (0336): an owner's links for people to join by, and the
	// public pages they join on, the code the credential.
	mux.Handle("GET /v1/invites", s.auth(s.inviteList))
	mux.Handle("POST /v1/invites", s.auth(s.inviteAdd))
	mux.Handle("DELETE /v1/invites/{id}", s.auth(s.inviteRevoke))
	mux.HandleFunc("GET /v1/register/{code}", s.registerRead)
	mux.HandleFunc("POST /v1/register", s.register)
	// The manager's own releases (0183).
	mux.Handle("GET /v1/system/update", s.auth(s.updateView))
	mux.Handle("GET /v1/system/health", s.auth(s.managerHealth))
	mux.Handle("POST /v1/system/update", s.auth(s.updateAsk))
	mux.Handle("GET /v1/advisor", s.auth(s.advice))
	mux.Handle("POST /v1/advisor/run", s.auth(s.adviseNow))
	mux.Handle("POST /v1/advisor/findings/{id}", s.auth(s.adviceState))
	mux.Handle("GET /v1/trees/{tree}", s.auth(s.tree))
	mux.Handle("GET /v1/trees/{tree}/nodes/{node}", s.auth(s.node))
	mux.Handle("GET /v1/aps", s.auth(s.aps))
	mux.Handle("GET /v1/aps/alive", s.auth(s.apsAlive))
	mux.Handle("GET /v1/alerts", s.auth(s.alertsList))
	mux.Handle("GET /v1/assurance", s.auth(s.assurance))
	mux.Handle("POST /v1/assurance/round", s.auth(s.askRound))
	mux.Handle("GET /v1/alerts/history", s.auth(s.alertHistory))
	mux.Handle("GET /v1/airspace", s.auth(s.airspaceList))
	mux.Handle("GET /v1/threats", s.auth(s.threatsList))
	mux.Handle("GET /v1/usage", s.auth(s.usageView))
	mux.Handle("GET /v1/clients", s.auth(s.clientList))
	mux.Handle("GET /v1/clients/{mac}", s.auth(s.clientJourney))
	mux.Handle("PUT /v1/clients/{mac}/name", s.auth(s.clientRename))
	mux.Handle("GET /v1/clients/{mac}/lookups", s.auth(s.clientLookups))
	mux.Handle("GET /v1/clients/{mac}/path", s.auth(s.clientPath))
	mux.Handle("GET /v1/places", s.auth(s.places))
	mux.Handle("GET /v1/map/tiles/{z}/{x}/{y}", s.auth(s.mapTile))
	mux.Handle("GET /v1/map/find", s.auth(s.mapFind))
	mux.Handle("GET /v1/map/suggest", s.auth(s.mapSuggest))
	mux.Handle("GET /v1/lookups/top", s.auth(s.lookupsTop))
	// What the APs' Bluetooth radios heard, kept (0158).
	mux.Handle("GET /v1/bluetooth", s.auth(s.bluetoothHeard))
	mux.Handle("GET /v1/bluetooth/devices/{mac}", s.auth(s.bluetoothDevice))
	mux.Handle("PUT /v1/bluetooth/devices/{mac}/name", s.auth(s.bluetoothRename))
	mux.Handle("GET /v1/client-names", s.auth(s.clientNames))
	mux.Handle("GET /v1/clients/{mac}/connections", s.auth(s.clientConnections))
	mux.Handle("GET /v1/clients/{mac}/trace", s.auth(s.traceRead))
	mux.Handle("GET /v1/clients/{mac}/trace/live", s.auth(s.traceLive))
	mux.Handle("POST /v1/clients/{mac}/trace", s.auth(s.traceStart))
	mux.Handle("DELETE /v1/clients/{mac}/trace", s.auth(s.traceStop))
	mux.Handle("GET /v1/connections/{id}/capture", s.auth(s.connectionCapture))
	mux.Handle("GET /v1/connections/{id}/capture/frames", s.auth(s.connectionFrames))
	// Captures people start (0148).
	mux.Handle("POST /v1/aps/{ap}/pcaps", s.auth(s.pcapStart))
	mux.Handle("GET /v1/pcaps", s.auth(s.pcapList))
	mux.Handle("GET /v1/pcaps/{id}/file", s.auth(s.pcapRead))
	mux.Handle("POST /v1/pcaps/{id}/stop", s.auth(s.pcapStop))
	mux.Handle("DELETE /v1/pcaps/{id}", s.auth(s.pcapDelete))
	mux.Handle("GET /v1/roaming", s.auth(s.roaming))
	mux.Handle("GET /v1/sticky", s.auth(s.sticky))
	mux.Handle("GET /v1/leaving", s.auth(s.leaving))
	mux.Handle("GET /v1/compatibility", s.auth(s.compatibility))
	mux.Handle("GET /v1/ssh", s.auth(s.sshKeys))
	mux.Handle("POST /v1/aps/{ap}/terminal", s.auth(s.termOpen))
	mux.Handle("GET /v1/terminal/{id}", s.auth(s.termRead))
	mux.Handle("POST /v1/terminal/{id}/in", s.auth(s.termType))
	mux.Handle("POST /v1/terminal/{id}/size", s.auth(s.termSize))
	mux.Handle("DELETE /v1/terminal/{id}", s.auth(s.termClose))
	mux.Handle("GET /v1/ap/terminal/{id}", s.apAuth(s.termAP))
	mux.Handle("POST /v1/ap/terminal/{id}", s.apAuth(s.termAPSaid))
	mux.Handle("GET /v1/steering", s.auth(s.steering))
	mux.Handle("GET /v1/power", s.auth(s.power))
	mux.Handle("GET /v1/minutes", s.auth(s.minutes))
	// Each radio's beacons over time, from the reports (0350).
	mux.Handle("GET /v1/beacons", s.auth(s.beaconsView))
	mux.Handle("GET /v1/noise", s.auth(s.noise))
	mux.Handle("GET /v1/live", s.auth(s.liveView))
	mux.Handle("GET /v1/recordings", s.auth(s.recordingsList))
	mux.Handle("GET /v1/recordings/{id}", s.auth(s.recordingRead))
	mux.Handle("POST /v1/recordings", s.auth(s.recordNow))
	mux.Handle("GET /v1/network", s.auth(s.network))
	mux.Handle("GET /v1/aps/{ap}/config", s.auth(s.apConfig))
	mux.Handle("GET /v1/aps/{ap}/history", s.auth(s.apHistory))
	mux.Handle("GET /v1/aps/{ap}/health", s.auth(s.apHealthView))
	mux.Handle("GET /v1/aps/{ap}/timeline", s.auth(s.apTimeline))
	mux.Handle("GET /v1/aps/{ap}/activity", s.auth(s.apActivity))
	mux.Handle("GET /v1/aps/{ap}/uplink", s.auth(s.apUplink))
	mux.Handle("GET /v1/aps/{ap}/actions", s.auth(s.actionList))
	mux.Handle("GET /v1/aps/{ap}/actions/{id}", s.auth(s.action))
	mux.Handle("POST /v1/aps/{ap}/actions", s.auth(s.addAction))
	mux.Handle("GET /v1/changes", s.auth(s.changes))
	// Changes that end themselves (0345): those to come, and one ended
	// now or its end moved.
	mux.Handle("GET /v1/ends", s.auth(s.endsList))
	mux.Handle("POST /v1/ends/{seq}", s.auth(s.endOne))
	mux.Handle("GET /v1/library", s.auth(s.library))
	mux.Handle("GET /v1/schema", s.auth(s.describe))
	mux.Handle("GET /v1/dhcp/relayed", s.auth(s.relayed))
	mux.Handle("GET /v1/detected", s.auth(s.detected))
	mux.Handle("GET /v1/keys", s.auth(s.keyList))
	mux.Handle("POST /v1/changes", s.auth(s.commit))
	mux.Handle("POST /v1/preview", s.auth(s.preview))
	mux.Handle("POST /v1/tokens", s.auth(s.issueToken))
	mux.Handle("DELETE /v1/tokens/{id}", s.auth(s.revokeToken))

	// The routes APs use (0033, 0038).
	mux.HandleFunc("POST /v1/enroll", s.enroll)
	mux.Handle("GET /v1/ap/config", s.apAuth(s.apPoll))
	mux.Handle("GET /v1/ap/agent", s.apAuth(s.agentManifest))
	mux.Handle("GET /v1/ap/agent/files/{sha256}", s.apAuth(s.agentFile))
	mux.Handle("GET /v1/agent/versions", s.auth(s.agentVersions))
	mux.Handle("POST /v1/ap/render", s.apAuth(s.render))
	mux.Handle("POST /v1/ap/applied", s.apAuth(s.applied))
	mux.Handle("POST /v1/ap/state", s.apAuth(s.state))
	mux.Handle("POST /v1/ap/facts", s.apAuth(s.apFacts))
	mux.Handle("POST /v1/ap/connections", s.apAuth(s.connections))
	mux.Handle("POST /v1/ap/captures", s.apAuth(s.capture))
	mux.Handle("GET /v1/ap/live", s.apAuth(s.apLive))
	mux.Handle("POST /v1/ap/live", s.apKnown(s.apLiveSaid))
	mux.Handle("GET /v1/ap/keys", s.apAuth(s.apKeys))
	mux.Handle("GET /v1/ap/ft", s.apAuth(s.apFT))
	mux.Handle("GET /v1/ap/trust", s.apAuth(s.apTrust))
	mux.Handle("GET /v1/ap/actions", s.apAuth(s.apActions))
	mux.Handle("POST /v1/ap/actions/{id}", s.apAuth(s.apActionDone))
	mux.Handle("POST /v1/ap/pcaps/{id}", s.apAuth(s.pcapPiece))

	// Installing from the manager, before an AP has a token (0083).
	mux.HandleFunc("GET /install", s.installScript)
	mux.HandleFunc("GET /install/manager.crt", s.installCert)
	mux.HandleFunc("GET /install/manifest", s.installManifest)
	mux.HandleFunc("GET /install/files/{sha256}", s.installFile)
	return slowLog(mux)
}

// slowAfter is how long an answer may take before the log says so (0211).
const slowAfter = time.Second

// slowLog says in the log each answer the API took slowAfter or more to give
// (0211): its method, path, status and milliseconds, so a page grown slow
// can be found by what it asks. Answers that do not end, the live lines and
// a terminal's, are meant to last, and are left out; so is one asked to wait,
// as an AP's key agent waits for its keys to change (meantToWait).
func slowLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		took := time.Since(start)
		if took < slowAfter || sw.waits {
			return
		}
		switch w.Header().Get("Content-Type") {
		case "application/x-ndjson", "application/octet-stream":
			return
		}
		slog.Warn("slow answer", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", took.Milliseconds())
		// And kept, for the manager's health (0219).
		keepSlow(slowAnswer{At: start, Method: r.Method, Path: r.URL.Path, Status: sw.status, MS: took.Milliseconds()})
	})
}

// statusWriter keeps the status an answer was given. Unwrap lets a
// ResponseController flush and set deadlines on the writer beneath.
type statusWriter struct {
	http.ResponseWriter
	status int
	// waits is an answer asked to wait, which is not slow for it.
	waits bool
}

// meantToWait says an answer was asked to wait, a long poll, so the slow log
// leaves it out however long it takes.
func meantToWait(w http.ResponseWriter) {
	for {
		switch x := w.(type) {
		case *statusWriter:
			x.waits = true
			return
		case interface{ Unwrap() http.ResponseWriter }:
			w = x.Unwrap()
		default:
			return
		}
	}
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// call is one authenticated request: who is asking, and the state they see.
type call struct {
	actor access.AccountID
	state *change.State
	// session is the sign-in session the request came with, or nil for a
	// bearer token (0176).
	session *sessions.Session
	// held is what a request the manager made of itself, for a folder's
	// advisor, is held to (0212); nil for any other.
	held *folderHold
}

type handler func(w http.ResponseWriter, r *http.Request, c call) error

// auth lets a request through as the account its bearer token or its
// session cookie names (0176). One made with the cookie that changes
// anything must carry SessionHeader; and with a temporary password, it may
// only find out who it is and change the password.
func (s *Server) auth(h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, code, msg := s.authenticate(w, r)
		if code < 0 {
			return // answered already
		}
		if code != 0 {
			writeError(w, code, msg)
			return
		}
		if c.session != nil {
			if !safeMethod(r.Method) && r.Header.Get(SessionHeader) == "" {
				writeError(w, http.StatusForbidden, "a change made with the session cookie carries the "+SessionHeader+" header")
				return
			}
			if acc, _ := c.state.Access.Account(c.actor); acc.MustChange && !mayBeforeChange(r) {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "change your password first", "must_change": true})
				return
			}
		}
		if err := h(w, r, c); err != nil {
			s.fail(w, r, string(c.actor), err)
		}
	})
}

// fail answers with the error's status. An unexpected error is logged, and
// the caller told only that something went wrong.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, who string, err error) {
	code := status(err)
	if code == http.StatusInternalServerError {
		slog.Error("api", "method", r.Method, "path", r.URL.Path, "caller", who, "err", err)
		writeError(w, code, "internal error")
		return
	}
	writeError(w, code, err.Error())
}

// apiError is an error with its own status.
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return e.msg }

var errNotFound = &apiError{http.StatusNotFound, "not found"}

func badRequest(format string, args ...any) error {
	return &apiError{http.StatusBadRequest, fmt.Sprintf(format, args...)}
}

func status(err error) int {
	var ae *apiError
	var le *hierarchy.LockedError
	var be *changelog.APBreakError
	var fe *schema.FieldError
	var ce *hierarchy.NetworkConflictError
	var pe *change.TemplatePickError
	switch {
	case errors.As(err, &ae):
		return ae.code
	case errors.Is(err, change.ErrForbidden), errors.Is(err, change.ErrUnknownActor):
		return http.StatusForbidden
	case errors.Is(err, change.ErrFull):
		return http.StatusServiceUnavailable
	case errors.Is(err, hierarchy.ErrNotFound), errors.Is(err, access.ErrNoAccount), errors.Is(err, change.ErrNoEnd),
		errors.Is(err, access.ErrNoToken), errors.Is(err, access.ErrNoGrant),
		errors.Is(err, access.ErrNoInvite), errors.Is(err, access.ErrBadInvite):
		return http.StatusNotFound
	case errors.Is(err, access.ErrInviteExpired), errors.Is(err, access.ErrInviteMaker):
		return http.StatusGone
	case errors.Is(err, access.ErrInviteUsed), errors.Is(err, access.ErrInviteRevoked):
		return http.StatusConflict
	case errors.Is(err, access.ErrInviteGives), errors.Is(err, change.ErrNoExpiry), errors.Is(err, change.ErrBadNote), errors.Is(err, change.ErrBadName):
		return http.StatusBadRequest
	case errors.Is(err, library.ErrNoConcentrator), errors.Is(err, library.ErrNoVNI), errors.Is(err, keys.ErrNoKey),
		errors.Is(err, library.ErrNoTemplate):
		return http.StatusNotFound
	case errors.Is(err, library.ErrTemplateExists):
		return http.StatusConflict
	case errors.Is(err, change.ErrBadKey):
		return http.StatusBadRequest
	case errors.Is(err, keys.ErrKeyExists), errors.Is(err, keys.ErrTooMany):
		return http.StatusConflict
	case errors.As(err, &le), errors.As(err, &be), errors.As(err, &ce), errors.Is(err, change.ErrInUse),
		errors.Is(err, hierarchy.ErrExists), errors.Is(err, access.ErrExists), errors.Is(err, access.ErrRevoked),
		errors.Is(err, hierarchy.ErrNotEmpty),
		errors.Is(err, access.ErrLastOwner), errors.Is(err, access.ErrRemoved), errors.Is(err, access.ErrInactive), errors.Is(err, access.ErrNoChange):
		return http.StatusConflict
	case errors.Is(err, change.ErrNoPassword), errors.Is(err, change.ErrNoOwner),
		errors.Is(err, access.ErrBadPasswordArg), errors.Is(err, access.ErrPasswordRules):
		return http.StatusBadRequest
	case errors.As(err, &fe),
		errors.Is(err, change.ErrUnknownKind), errors.Is(err, change.ErrUnknownTree),
		errors.Is(err, change.ErrNoValue), errors.Is(err, change.ErrNoNode), errors.Is(err, change.ErrNoPath), errors.Is(err, change.ErrTwoForms),
		errors.Is(err, change.ErrBadBatch), errors.Is(err, change.ErrNoAccount), errors.Is(err, change.ErrNoTokenID), errors.Is(err, change.ErrUseAssign),
		errors.Is(err, change.ErrBadUntil), errors.Is(err, change.ErrUseEnd),
		errors.Is(err, change.ErrNotAFolder), errors.Is(err, access.ErrBadRole), errors.Is(err, change.ErrNoConcID),
		errors.Is(err, library.ErrBadVNI), errors.Is(err, library.ErrNoLabel), errors.Is(err, change.ErrBuiltins), errors.Is(err, change.ErrBadBuiltins),
		errors.Is(err, change.ErrReserved), errors.Is(err, hierarchy.ErrIsolated),
		errors.Is(err, hierarchy.ErrBadParent), errors.Is(err, hierarchy.ErrAPsNotHere), errors.Is(err, hierarchy.ErrMoveRoot),
		errors.Is(err, hierarchy.ErrCycle), errors.Is(err, hierarchy.ErrBreakRoot), errors.Is(err, hierarchy.ErrBroken),
		errors.Is(err, hierarchy.ErrNotSetHere), errors.Is(err, hierarchy.ErrLockOnValue), errors.Is(err, hierarchy.ErrNotLocked),
		errors.Is(err, hierarchy.ErrNotAnAP), errors.Is(err, hierarchy.ErrNotAFolder), errors.Is(err, access.ErrBadTokenArg),
		errors.As(err, &pe), errors.Is(err, change.ErrNoTemplateID), errors.Is(err, change.ErrTemplateLevel),
		errors.Is(err, change.ErrTemplateField), errors.Is(err, change.ErrTemplateName),
		errors.Is(err, library.ErrTemplateID), errors.Is(err, library.ErrBoard), errors.Is(err, library.ErrNoBoards),
		errors.Is(err, library.ErrNotSet):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: encoding response", "err", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// readJSON decodes a request body strictly: unknown fields are errors.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("request body: %v", err)
	}
	return nil
}

// root points a visitor, or a browser, at what is here.
func root(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "aeolus", "api": "/v1", "health": "/healthz", "mcp": "/mcp",
		"docs": "https://github.com/WiFiGUY512/aeolus/blob/main/docs/api.md",
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seq": s.log.Seq()})
}

// mask replaces sealed secrets, wherever they sit in a value, with a marker.
func mask(v any) any {
	if secret.IsSealed(v) {
		return map[string]any{"sealed": true}
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = mask(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = mask(e)
		}
		return out
	}
	return v
}

// reveal opens a sealed value with the manager's key, for checks only.
func (s *Server) reveal(path string, v any) (any, error) {
	if s.box == nil {
		return v, nil
	}
	return s.box.Open(path, v)
}

// may says whether the caller may do x on a node (0177): never on one
// that does not exist.
func may(c call, tree change.TreeName, t *hierarchy.Tree, node hierarchy.NodeID, x access.Right) bool {
	chain := t.Ancestry(node)
	if chain == nil {
		return false
	}
	// A folder's advisor reads its folder alone, whatever its roles (0212).
	if c.held != nil && !c.held.covers(tree, chain) {
		return false
	}
	return c.state.Access.May(c.actor, string(tree), chain, x)
}

func treeOf(state *change.State, name string) (change.TreeName, *hierarchy.Tree, error) {
	switch change.TreeName(name) {
	case change.Locations:
		return change.Locations, state.Org.Locations, nil
	case change.Services:
		return change.Services, state.Org.Services, nil
	}
	return "", nil, errNotFound
}
