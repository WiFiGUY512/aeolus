package api

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestManagerSSHKey: the manager's own SSH key (0197) comes beside every
// ready config, is a key OpenSSH reads, is the same each time, and is what
// GET /v1/ssh says; its private file is one OpenSSH reads too, of the same key.
func TestManagerSSHKey(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()

	code, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code != 200 {
		t.Fatalf("poll: %d", code)
	}
	key, _ := body["ssh"].(map[string]any)["manager"].(string)
	pub, comment, _, rest, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil || len(rest) != 0 || comment != ManagerKeyComment || pub.Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("the manager's key %q: %v, comment %q", key, err, comment)
	}
	if _, _, again := f.apDo("GET", "/v1/ap/config", token, nil, nil); again["ssh"].(map[string]any)["manager"] != key {
		t.Fatalf("the key changed between polls")
	}
	if code, b := f.do("GET", "/v1/ssh", "griff", nil); code != 200 || b["manager"] != key {
		t.Fatalf("GET /v1/ssh: %d %v, want %q", code, b, key)
	}

	private, public, err := f.api.ManagerSSHFiles()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(private)
	if err != nil {
		t.Fatalf("the private file: %v", err)
	}
	if string(ssh.MarshalAuthorizedKey(signer.PublicKey())) != string(ssh.MarshalAuthorizedKey(pub)) {
		t.Fatalf("the private file is not of the key the APs are sent")
	}
	if strings.TrimSpace(string(public)) != key {
		t.Fatalf("the public file %q, want %q", public, key)
	}
}
