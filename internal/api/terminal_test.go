package api

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// dropbear stands in for an AP's dropbear: an SSH server that lets in the
// manager's key alone, gives a pty and a shell, and echoes what it is sent.
// It says its host key as an authorized_keys line.
func dropbear(t *testing.T, manager ssh.PublicKey) (net.Listener, string) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	host, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if c.User() == "root" && bytes.Equal(k.Marshal(), manager.Marshal()) {
			return nil, nil
		}
		return nil, io.EOF
	}}
	cfg.AddHostKey(host)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, _ := nch.Accept()
					go func() {
						for r := range creqs {
							r.Reply(r.Type == "pty-req" || r.Type == "shell" || r.Type == "window-change", nil)
						}
					}()
					go func() {
						defer ch.Close()
						ch.Write([]byte("# "))
						sc := bufio.NewScanner(ch)
						for sc.Scan() {
							if sc.Text() == "exit" {
								ch.SendRequest("exit-status", false, ssh.Marshal(struct{ C uint32 }{0}))
								return
							}
							ch.Write([]byte("echo:" + sc.Text() + "\r\n# "))
						}
					}()
				}
			}()
		}
	}()
	return ln, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(host.PublicKey())))
}

// fakeShell plays aeolus-shell: told a terminal's ID, it joins dropbear to
// the manager by a GET and POSTs of its own.
func fakeShell(t *testing.T, base, token, id string, db net.Listener, hostKey string) {
	conn, err := net.Dial("tcp", db.Addr().String())
	if err != nil {
		t.Error(err)
		return
	}
	defer conn.Close()
	req, _ := http.NewRequest("GET", base+"/v1/ap/terminal/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Aeolus-Host-Key", hostKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Errorf("the AP's GET: %v %v", err, res)
		return
	}
	defer res.Body.Close()
	go io.Copy(conn, res.Body)
	buf := make([]byte, 32<<10)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			post, _ := http.NewRequest("POST", base+"/v1/ap/terminal/"+id, bytes.NewReader(buf[:n]))
			post.Header.Set("Authorization", "Bearer "+token)
			r, err := http.DefaultClient.Do(post)
			if err != nil {
				return
			}
			r.Body.Close()
			if r.StatusCode != 204 {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// TestTerminal (0198): an admin opens a terminal on an AP that holds its
// line; the AP is told, joins its dropbear to the manager, and the manager
// speaks SSH to it with its own key, checking the host key the AP named;
// what is typed reaches the shell and what it prints comes back; the size
// is set; closing it ends it, and says so; an operator may not open one;
// the AP's actions say who opened it.
func TestTerminal(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	mpub, _ := ssh.NewPublicKey(f.api.ManagerSSHKey().Public())
	db, hostKey := dropbear(t, mpub)

	// The AP's line, as aeolus-live holds it, and what it does when told.
	lreq, _ := http.NewRequest("GET", f.url+"/v1/ap/live", nil)
	lreq.Header.Set("Authorization", "Bearer "+token)
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil || lres.StatusCode != 200 {
		t.Fatalf("the line: %v %v", err, lres)
	}
	defer lres.Body.Close()
	go func() {
		sc := bufio.NewScanner(lres.Body)
		for sc.Scan() {
			var m struct {
				Shell *liveShell `json:"shell"`
			}
			if json.Unmarshal(sc.Bytes(), &m) == nil && m.Shell != nil {
				go fakeShell(t, f.url, token, m.Shell.ID, db, hostKey)
			}
		}
	}()

	if code, b := f.do("POST", "/v1/aps/"+ap+"/terminal", "claude", map[string]any{"cols": 100, "rows": 30}); code != 403 {
		t.Fatalf("an operator opened a terminal: %d %v", code, b)
	}
	var id string
	for i := 0; ; i++ {
		code, b := f.do("POST", "/v1/aps/"+ap+"/terminal", "griff", map[string]any{"cols": 100, "rows": 30})
		if code == 200 {
			id = b["id"].(string)
			break
		}
		if code != 409 || i > 40 {
			t.Fatalf("open: %d %v", code, b)
		}
		time.Sleep(50 * time.Millisecond) // the line is still joining
	}

	// The browser's side: what the shell prints, as it prints it.
	rreq, _ := http.NewRequest("GET", f.url+"/v1/terminal/"+id, nil)
	rreq.Header.Set("Authorization", "Bearer "+f.tokens["griff"])
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil || rres.StatusCode != 200 {
		t.Fatalf("read: %v %v", err, rres)
	}
	defer rres.Body.Close()
	got := make(chan string, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := rres.Body.Read(buf)
			if n > 0 {
				got <- string(buf[:n])
			}
			if err != nil {
				close(got)
				return
			}
		}
	}()
	seen := ""
	wait := func(want string) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for !strings.Contains(seen, want) {
			select {
			case s, ok := <-got:
				if !ok {
					t.Fatalf("the shell ended before %q; it printed %q", want, seen)
				}
				seen += s
			case <-deadline:
				t.Fatalf("no %q; the shell printed %q", want, seen)
			}
		}
	}
	wait("# ")
	if code, b := f.doRaw("POST", "/v1/terminal/"+id+"/in", "griff", []byte("hello\n")); code != 204 {
		t.Fatalf("type: %d %s", code, b)
	}
	wait("echo:hello")
	if code, b := f.doRaw("POST", "/v1/terminal/"+id+"/size", "griff", []byte(`{"cols":120,"rows":40}`)); code != 204 {
		t.Fatalf("size: %d %s", code, b)
	}
	// Another account may not touch it.
	if code, _ := f.doRaw("POST", "/v1/terminal/"+id+"/in", "claude", []byte("x\n")); code != 404 {
		t.Fatalf("another account typed: %d", code)
	}
	if code, b := f.doRaw("DELETE", "/v1/terminal/"+id, "griff", nil); code != 204 {
		t.Fatalf("close: %d %s", code, b)
	}
	wait("[Aeolus: closed by griff]")

	code, b := f.do("GET", "/v1/aps/"+ap+"/actions", "griff", nil)
	if code != 200 || !strings.Contains(string(mustJSON(b)), "a terminal was opened, by griff") {
		t.Fatalf("the AP's actions: %d %v", code, b)
	}
}

// doRaw sends a request with a body as it is, and returns the status and
// the answer as it is.
func (f *fixture) doRaw(method, path, as string, body []byte) (int, string) {
	f.t.Helper()
	req, err := http.NewRequest(method, f.url+path, bytes.NewReader(body))
	must(f.t, err)
	if as != "" {
		req.Header.Set("Authorization", "Bearer "+f.tokens[as])
	}
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
