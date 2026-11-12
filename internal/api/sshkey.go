package api

import (
	"bytes"
	"crypto/ed25519"
	"encoding/pem"
	"net/http"

	"golang.org/x/crypto/ssh"
)

// ManagerKeyComment names the manager's own SSH key in an AP's
// authorized_keys (0197).
const ManagerKeyComment = "aeolus-manager"

// ManagerSSHKey is the manager's own SSH key (0197): the one every AP lets in,
// beside those set in system.ssh_keys, so the manager, and whoever runs it,
// can reach any AP. It is made from the manager's secret key, so it is the
// same after a restart and is kept nowhere it was not already.
func (s *Server) ManagerSSHKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(s.box.Derive("ssh manager")[:ed25519.SeedSize])
}

// managerSSHPublic is the manager's key as an authorized_keys line.
func (s *Server) managerSSHPublic() string {
	pub, err := ssh.NewPublicKey(s.ManagerSSHKey().Public())
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(pub))) + " " + ManagerKeyComment
}

// ManagerSSHFiles are the manager's key as OpenSSH writes one, private and
// public, for whoever runs the manager to reach APs with (ssh -i).
func (s *Server) ManagerSSHFiles() (private, public []byte, err error) {
	block, err := ssh.MarshalPrivateKey(s.ManagerSSHKey(), ManagerKeyComment)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(block), []byte(s.managerSSHPublic() + "\n"), nil
}

// sshKeys answers GET /v1/ssh: the manager's own key, which every AP lets in.
func (s *Server) sshKeys(w http.ResponseWriter, _ *http.Request, _ call) error {
	writeJSON(w, http.StatusOK, map[string]any{"manager": s.managerSSHPublic()})
	return nil
}
