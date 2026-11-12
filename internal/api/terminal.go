package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"golang.org/x/crypto/ssh"
)

// A terminal on an AP (0198): a shell there as root, in a window of the
// UI. The AP opens every connection, as 0007 has it: told on its line
// (0145) that a terminal waits, aeolus-shell on the AP connects its own
// dropbear to the manager by two requests of its own, a GET whose answer
// carries what goes to dropbear and POSTs that carry what dropbear says.
// Over that the manager speaks SSH to the AP with its own key (0197),
// checking the AP's host key, which the AP names on its GET; and the
// browser has the shell by a GET whose answer is what the shell prints,
// and POSTs of what is typed.

const (
	termKind   = "terminal"       // as the AP's actions list it
	termAttach = 20 * time.Second // how long the AP has to open its side
	termIdle   = 30 * time.Minute // a terminal no one types in or reads from ends
	termMost   = 8 * time.Hour    // the longest a terminal lasts
	termKept   = time.Minute      // how long an ended one waits to be read
	termInMost = 64 << 10         // the most typed at once
	termOut    = 256              // chunks of output that wait for the browser
)

// termSession is one terminal.
type termSession struct {
	id     string
	ap     hierarchy.NodeID
	actor  access.AccountID
	opened time.Time

	toAP     chan []byte // what the SSH client sends, for the AP's GET
	apR      *io.PipeReader
	apW      *io.PipeWriter // what the AP posts, for the SSH client
	attached chan struct{}  // closed when the AP opens its side
	hostKey  string

	out  chan []byte   // what the shell prints, for the browser
	done chan struct{} // closed when the terminal ends

	mu      sync.Mutex
	why     string // why it ended
	ended   time.Time
	last    time.Time // when someone last typed, or it last printed
	ap1     bool      // the AP has opened its side
	reading bool      // a browser reads it now
	sess    *ssh.Session
	stdin   io.WriteCloser
	client  *ssh.Client
	cols    int
	rows    int
}

// terms is the terminals open, by ID.
type terms struct {
	mu sync.Mutex
	m  map[string]*termSession
}

func (t *terms) get(id string) *termSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[id]
}

func (t *terms) put(s *termSession) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]*termSession{}
	}
	t.m[s.id] = s
}

func (t *terms) drop(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, id)
}

// end ends the terminal once, saying why.
func (t *termSession) end(why string) {
	t.mu.Lock()
	if t.why != "" || isClosed(t.done) {
		t.mu.Unlock()
		return
	}
	t.why, t.ended = why, time.Now()
	stdin, sess, client := t.stdin, t.sess, t.client
	close(t.done)
	t.mu.Unlock()
	t.apW.CloseWithError(io.EOF)
	t.apR.Close()
	if stdin != nil {
		stdin.Close()
	}
	if sess != nil {
		sess.Close()
	}
	if client != nil {
		client.Close()
	}
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func (t *termSession) touch() {
	t.mu.Lock()
	t.last = time.Now()
	t.mu.Unlock()
}

// apConn is the AP's side as the SSH client sees it: what it reads came
// in the AP's POSTs; what it writes goes out on the AP's GET.
type apConn struct{ t *termSession }

func (c apConn) Read(b []byte) (int, error) { return c.t.apR.Read(b) }

func (c apConn) Write(b []byte) (int, error) {
	p := append([]byte(nil), b...)
	select {
	case c.t.toAP <- p:
		return len(b), nil
	case <-c.t.done:
		return 0, net.ErrClosed
	}
}

func (c apConn) Close() error                       { c.t.end("closed"); return nil }
func (c apConn) LocalAddr() net.Addr                { return termAddr("manager") }
func (c apConn) RemoteAddr() net.Addr               { return termAddr(c.t.ap) }
func (c apConn) SetDeadline(t time.Time) error      { return nil }
func (c apConn) SetReadDeadline(t time.Time) error  { return nil }
func (c apConn) SetWriteDeadline(t time.Time) error { return nil }

type termAddr string

func (a termAddr) Network() string { return "aeolus-terminal" }
func (a termAddr) String() string  { return string(a) }

// liveShell is a terminal to open, as an AP's line says it (0198).
type liveShell struct {
	ID string `json:"id"`
}

// termOpen opens a terminal on an AP: POST /v1/aps/{ap}/terminal,
// {cols, rows}. It needs the shell right there, an admin's (0198), and an
// AP that holds its line (0145). The answer is {id}; the shell comes on
// GET /v1/terminal/{id}.
func (s *Server) termOpen(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || !may(c, change.Locations, t, id, access.View) {
		return errNotFound
	}
	if !may(c, change.Locations, t, id, access.Shell) {
		return &apiError{http.StatusForbidden, "a terminal is a shell on the AP as root, which needs the shell right on it: an admin's (0198)"}
	}
	var body struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Cols < 20 || body.Cols > 500 || body.Rows < 5 || body.Rows > 300 {
		body.Cols, body.Rows = 80, 24
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	pr, pw := io.Pipe()
	now := time.Now()
	ts := &termSession{
		id: hex.EncodeToString(raw), ap: id, actor: c.actor, opened: now, last: now,
		toAP: make(chan []byte, 64), apR: pr, apW: pw, attached: make(chan struct{}),
		out: make(chan []byte, termOut), done: make(chan struct{}),
		cols: body.Cols, rows: body.Rows,
	}
	b, _ := json.Marshal(liveSay{Now: msOf(now), Shell: &liveShell{ID: ts.id}})
	if !s.live.tellOne(id, append(b, '\n')) {
		return &apiError{http.StatusConflict, "the AP holds no line to the manager (0145), so it cannot be asked to open a terminal: it is offline, or its agent is older than 0198"}
	}
	s.terms.put(ts)
	go s.termRun(ts)
	// The AP's actions say who opened a terminal there, and when: a
	// finished one, since the agent has nothing of it to do at its poll.
	if a, err := s.conds.AddAction(id, termKind, "", string(c.actor)); err == nil {
		_, _ = s.conds.FinishAction(id, a.ID, true, "a terminal was opened, by "+string(c.actor))
	}
	slog.Info("terminal opened", "ap", id, "id", ts.id, "by", c.actor)
	writeJSON(w, http.StatusOK, map[string]any{"id": ts.id})
	return nil
}

// termRun waits for the AP to open its side, speaks SSH to its dropbear,
// and keeps the shell until it ends.
func (s *Server) termRun(t *termSession) {
	defer func() {
		t.end("the shell ended")
		slog.Info("terminal ended", "ap", t.ap, "id", t.id, "by", t.actor, "why", t.why, "minutes", int(time.Since(t.opened).Minutes()))
		time.AfterFunc(termKept, func() { s.terms.drop(t.id) })
	}()
	select {
	case <-t.attached:
	case <-t.done:
		return
	case <-time.After(termAttach):
		t.end("the AP did not open its side within 20 seconds: its agent may be older than 0198, or it cannot reach the manager")
		return
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(t.hostKey))
	if err != nil {
		t.end("the AP named no host key to check it by")
		return
	}
	signer, err := ssh.NewSignerFromKey(s.ManagerSSHKey())
	if err != nil {
		t.end("the manager's key: " + err.Error())
		return
	}
	conn, chans, reqs, err := ssh.NewClientConn(apConn{t}, string(t.ap), &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(key),
	})
	if err != nil {
		t.end("SSH to the AP: " + err.Error() + " (does the AP let the manager's key in? 0197)")
		return
	}
	client := ssh.NewClient(conn, chans, reqs)
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		t.end("a session on the AP: " + err.Error())
		return
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 115200, ssh.TTY_OP_OSPEED: 115200}
	t.mu.Lock()
	cols, rows := t.cols, t.rows
	t.mu.Unlock()
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		t.end("a terminal on the AP: " + err.Error())
		return
	}
	stdin, err1 := sess.StdinPipe()
	stdout, err2 := sess.StdoutPipe()
	if err := errors.Join(err1, err2, sess.Shell()); err != nil {
		sess.Close()
		client.Close()
		t.end("a shell on the AP: " + err.Error())
		return
	}
	t.mu.Lock()
	t.sess, t.stdin, t.client = sess, stdin, client
	t.mu.Unlock()
	if isClosed(t.done) {
		return
	}
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				t.touch()
				select {
				case t.out <- append([]byte(nil), buf[:n]...):
				case <-t.done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// It ends when the shell does, or when it has been left too long.
	watch := time.NewTicker(time.Minute)
	defer watch.Stop()
	ended := make(chan struct{})
	go func() {
		sess.Wait()
		close(ended)
	}()
	for {
		select {
		case <-ended:
			// What it printed last goes before the end is said.
			time.Sleep(200 * time.Millisecond)
			t.end("the shell ended")
			return
		case <-t.done:
			return
		case now := <-watch.C:
			t.mu.Lock()
			idle, long := now.Sub(t.last) > termIdle, now.Sub(t.opened) > termMost
			t.mu.Unlock()
			if idle {
				t.end(fmt.Sprintf("nothing was typed or printed for %d minutes", int(termIdle.Minutes())))
			} else if long {
				t.end(fmt.Sprintf("a terminal lasts %d hours at most", int(termMost.Hours())))
			}
		}
	}
}

// termOf is the terminal by its ID, for the account that opened it alone.
func (s *Server) termOf(r *http.Request, c call) (*termSession, error) {
	t := s.terms.get(r.PathValue("id"))
	if t == nil || t.actor != c.actor {
		return nil, errNotFound
	}
	return t, nil
}

// termRead is the shell for the browser: GET /v1/terminal/{id}, an answer
// that does not end until the terminal does, of what the shell prints, as
// it prints it. Its end is said last, as a line of its own. One browser
// reads it at once: a second takes it over.
func (s *Server) termRead(w http.ResponseWriter, r *http.Request, c call) error {
	t, err := s.termOf(r, c)
	if err != nil {
		return err
	}
	write, err := stream(w, "application/octet-stream")
	if err != nil {
		return err
	}
	if write(nil) != nil {
		return nil
	}
	for {
		select {
		case <-r.Context().Done():
			return nil
		case b := <-t.out:
			t.touch()
			if write(b) != nil {
				// What did not get there is put back for the next read.
				select {
				case t.out <- b:
				default:
				}
				return nil
			}
		case <-t.done:
			for {
				select {
				case b := <-t.out:
					if write(b) != nil {
						return nil
					}
					continue
				default:
				}
				break
			}
			t.mu.Lock()
			why := t.why
			t.mu.Unlock()
			_ = write([]byte("\r\n\x1b[2m[Aeolus: " + why + "]\x1b[0m\r\n"))
			return nil
		}
	}
}

// termType sends what is typed: POST /v1/terminal/{id}/in, the bytes as
// the body.
func (s *Server) termType(w http.ResponseWriter, r *http.Request, c call) error {
	t, err := s.termOf(r, c)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, termInMost+1))
	if err != nil {
		return err
	}
	if len(b) > termInMost {
		return badRequest("at most %d bytes at a time", termInMost)
	}
	t.mu.Lock()
	stdin := t.stdin
	t.mu.Unlock()
	if stdin == nil || isClosed(t.done) {
		return &apiError{http.StatusConflict, "the shell is not open"}
	}
	t.touch()
	if _, err := stdin.Write(b); err != nil {
		return &apiError{http.StatusConflict, "the shell is not open"}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// termSize sets the terminal's size: POST /v1/terminal/{id}/size,
// {cols, rows}.
func (s *Server) termSize(w http.ResponseWriter, r *http.Request, c call) error {
	t, err := s.termOf(r, c)
	if err != nil {
		return err
	}
	var body struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Cols < 20 || body.Cols > 500 || body.Rows < 5 || body.Rows > 300 {
		return badRequest("cols from 20 to 500, rows from 5 to 300")
	}
	t.mu.Lock()
	t.cols, t.rows = body.Cols, body.Rows
	sess := t.sess
	t.mu.Unlock()
	if sess != nil {
		_ = sess.WindowChange(body.Rows, body.Cols)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// termClose ends a terminal: DELETE /v1/terminal/{id}.
func (s *Server) termClose(w http.ResponseWriter, r *http.Request, c call) error {
	t, err := s.termOf(r, c)
	if err != nil {
		return err
	}
	t.end("closed by " + string(c.actor))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// termAP is the AP's side of a terminal opened on it: GET
// /v1/ap/terminal/{id}, naming its dropbear host key in the Aeolus-Host-Key
// header as an authorized_keys line, an answer that does not end until the
// terminal does, of what goes to its dropbear.
func (s *Server) termAP(w http.ResponseWriter, r *http.Request, c apCall) error {
	t := s.terms.get(r.PathValue("id"))
	if t == nil || t.ap != c.ap {
		return errNotFound
	}
	t.mu.Lock()
	if t.ap1 || isClosed(t.done) {
		t.mu.Unlock()
		return &apiError{http.StatusConflict, "this terminal's AP side is open already, or it has ended"}
	}
	t.ap1, t.hostKey = true, r.Header.Get("Aeolus-Host-Key")
	t.mu.Unlock()
	write, err := stream(w, "application/octet-stream")
	if err != nil {
		t.end("the AP's side: " + err.Error())
		return err
	}
	if write(nil) != nil {
		t.end("the AP's side went")
		return nil
	}
	close(t.attached)
	for {
		select {
		case <-r.Context().Done():
			t.end("the AP's side went")
			return nil
		case <-t.done:
			return nil
		case b := <-t.toAP:
			if write(b) != nil {
				t.end("the AP's side went")
				return nil
			}
		}
	}
}

// termAPSaid is what the AP's dropbear says: POST /v1/ap/terminal/{id},
// the bytes as the body, in order, one at a time.
func (s *Server) termAPSaid(w http.ResponseWriter, r *http.Request, c apCall) error {
	t := s.terms.get(r.PathValue("id"))
	if t == nil || t.ap != c.ap || isClosed(t.done) {
		return errNotFound
	}
	if _, err := io.Copy(t.apW, r.Body); err != nil {
		return &apiError{http.StatusGone, "the terminal has ended"}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// stream answers with an answer that does not end, each write sent at
// once. A nil write sends the headers alone.
func stream(w http.ResponseWriter, kind string) (func([]byte) error, error) {
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, err
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return func(b []byte) error {
		if err := rc.SetWriteDeadline(time.Now().Add(liveBeat + liveSlow)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if len(b) > 0 {
			if _, err := w.Write(b); err != nil {
				return err
			}
		}
		return rc.Flush()
	}, nil
}
