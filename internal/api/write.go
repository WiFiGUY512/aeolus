package api

import (
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/changelog"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// NewKinds gives each kind of AP adopted that no template is for a template
// of its own, empty, at the Org, in the manager's name (0085). The manager
// runs it when it starts, and after each change, which may have adopted one.
func NewKinds(log *changelog.Log) {
	for _, op := range change.NewKinds(log.Snapshot()) {
		if _, err := log.Commit(change.SystemActor, "a new kind of AP adopted (0085)", op); err != nil {
			slog.Warn("the template of a new kind of AP", "board", op.Boards, "err", err)
		}
	}
}

type changeRequest struct {
	Op     change.Op `json:"op"`
	Reason string    `json:"reason"`
}

// prepare readies a submitted change: kinds with their own route are sent
// there, and set values are validated and, for secrets, sealed before they go
// anywhere near the log (0027).
func (s *Server) prepare(op change.Op) (change.Op, error) {
	switch op.Kind {
	case change.AddKey, change.SetKey, change.RemoveKey:
		return s.prepareKey(s.log.Snapshot(), op, time.Now())
	case change.CreateOrg:
		return op, badRequest("the Org is created on the manager host with aeolus init")
	case change.IssueToken:
		return op, badRequest("tokens are issued with POST /v1/tokens")
	case change.AddInvite:
		// Its secret is the manager's to make, and to answer once (0336).
		return op, badRequest("invites are made with POST /v1/invites")
	case change.UseInvite:
		return op, badRequest("an invite is used by the one it is for, at its link, through POST /v1/register")
	case change.SetPassword:
		// A password is never sent as a change, hashed or not (0176).
		return op, badRequest("a password is set by its own route, never as a change; on the manager host, aeolus password sets a temporary one")
	case change.SetConcentrator, change.RemoveConcentrator, change.SetVNI, change.RemoveVNI:
		return op, badRequest("the library's concentrators are retired (0085): a tunnel is set in Interfaces, as concentrators.<name> on a folder or AP (0055)")
	case change.AddTemplate, change.SetTemplate:
		// A template's values are Locations fields, checked and sealed as
		// they are (0085); an imported template comes with them (0090).
		if op.Kind == change.AddTemplate && len(op.Values) == 0 {
			return op, nil
		}
		if len(op.Values) == 0 {
			v, err := s.prepareValue(change.Locations, op.Path, op.Value)
			op.Value = v
			return op, err
		}
		values := make(map[hierarchy.Path]json.RawMessage, len(op.Values))
		for p, raw := range op.Values {
			v, err := s.prepareValue(change.Locations, p, raw)
			if err != nil {
				return op, err
			}
			values[p] = v
		}
		op.Values = values
	case change.Batch:
		// Each of a batch's changes is readied as it would be made alone:
		// its values checked and its secrets sealed (0203). What a batch may
		// hold is the change package's to say (0320).
		ops := make([]change.Op, len(op.Ops))
		for i, sub := range op.Ops {
			if !change.Batchable(sub.Kind) {
				return op, badRequest("change %d of the batch is %s, which a batch does not make", i+1, sub.Kind)
			}
			p, err := s.prepare(sub)
			if err != nil {
				return op, err
			}
			ops[i] = p
		}
		op.Ops = ops
	case change.Set:
		if len(op.Values) == 0 {
			v, err := s.prepareValue(op.Tree, op.Path, op.Value)
			op.Value = v
			return op, err
		}
		values := make(map[hierarchy.Path]json.RawMessage, len(op.Values))
		for p, raw := range op.Values {
			v, err := s.prepareValue(op.Tree, p, raw)
			if err != nil {
				return op, err
			}
			values[p] = v
		}
		op.Values = values
	}
	return op, nil
}

func (s *Server) prepareValue(tree change.TreeName, p hierarchy.Path, raw json.RawMessage) (json.RawMessage, error) {
	if tree == change.Locations && p == hierarchy.ServicesPath {
		return raw, nil // refused by Apply: use assign-services
	}
	// A pin to an agent release the manager keeps (0079), checked here, when
	// it is set, rather than in the log's own check, which replays.
	if tree == change.Locations && p == AgentPath {
		if err := s.checkAgentPin(raw); err != nil {
			return raw, err
		}
	}
	// A remote speed server by a name the list has not (0134), as one
	// mistyped, would measure nothing and say nothing: refused here, when
	// it is set, as the schema takes any name, one a later manager's list
	// has among them.
	if tree == change.Locations && p == "system.speed_server_remote" {
		var v string
		if err := json.Unmarshal(raw, &v); err == nil && remoteSpeed(v) == nil {
			return raw, badRequest("system.speed_server_remote: %q is no public speed server in the list (%s), nor one of your own as http://host or http://host:port", v, strings.Join(slices.Sorted(maps.Keys(publicSpeed)), ", "))
		}
	}
	return s.schema.Prepare(p, raw, s.box)
}

// checks runs the whole-config check for the given APs and returns the ones
// that fail, with their problems (0029).
func (s *Server) checks(state *change.State, aps []hierarchy.NodeID) map[hierarchy.NodeID][]string {
	out := map[hierarchy.NodeID][]string{}
	for _, ap := range aps {
		if p := s.apProblems(state, ap); len(p) > 0 {
			out[ap] = p
		}
	}
	return out
}

// commit makes one change (0026).
func (s *Server) commit(w http.ResponseWriter, r *http.Request, c call) error {
	var req changeRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	op, err := s.prepare(req.Op)
	if err != nil {
		return err
	}
	e, err := s.log.Commit(string(c.actor), req.Reason, op)
	if err != nil {
		return err
	}
	after := s.log.Snapshot()
	reversioned := s.reversioned(after, e.Seq)
	if op.Kind == change.Move {
		NewKinds(s.log) // an adoption, perhaps; its template changes no AP's config
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"change":      viewEntry(e),
		"reversioned": reversioned,
		"checks":      s.checks(after, reversioned),
	})
	return nil
}

// preview runs a change with every check and records nothing (0026).
func (s *Server) preview(w http.ResponseWriter, r *http.Request, c call) error {
	var req changeRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	op, err := s.prepare(req.Op)
	if err != nil {
		return err
	}
	state, eff, changed, err := s.log.Preview(string(c.actor), op)
	if err != nil {
		return err
	}
	if changed == nil {
		changed = []hierarchy.NodeID{}
	}
	eff = viewEffect(eff)
	writeJSON(w, http.StatusOK, map[string]any{
		"effect":      eff,
		"reversioned": changed,
		"checks":      s.checks(state, changed),
		"resolved":    resolvedAfter(state, op),
	})
	return nil
}

// resolvedAfter shows how the fields a set or unset touches would resolve
// at its node once it is made, so a preview can say what a field becomes;
// a field nothing would set any more is null (0046). A batch's is each of
// its changes', in order, with its tree and node (0203).
func resolvedAfter(state *change.State, op change.Op) any {
	if op.Kind == change.Batch {
		out := make([]map[string]any, len(op.Ops))
		for i, sub := range op.Ops {
			out[i] = map[string]any{"tree": sub.Tree, "node": sub.Node, "resolved": resolvedAfter(state, sub)}
		}
		return out
	}
	var paths []hierarchy.Path
	switch op.Kind {
	case change.Set:
		for _, f := range op.Fields() {
			paths = append(paths, f.Path)
		}
	case change.Unset:
		paths = op.Unsets()
	default:
		return nil
	}
	_, t, err := treeOf(state, string(op.Tree))
	if err != nil {
		return nil
	}
	out := make(map[hierarchy.Path]*resolvedView, len(paths))
	for _, p := range paths {
		out[p] = nil
		if r, ok := t.Resolve(op.Node, p); ok {
			v := viewResolved(r)
			out[p] = &v
		}
	}
	return out
}

// issueToken issues a token for an account and returns it once. Only its ID
// and hash are logged (0024, 0030).
func (s *Server) issueToken(w http.ResponseWriter, r *http.Request, c call) error {
	var req struct {
		Account access.AccountID `json:"account"`
		Reason  string           `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Account == "" {
		req.Account = c.actor
	}
	plain, id, hash, err := access.NewToken()
	if err != nil {
		return err
	}
	op := change.Op{Kind: change.IssueToken, Account: req.Account, TokenID: id, TokenHash: hash}
	if _, err := s.log.Commit(string(c.actor), req.Reason, op); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": req.Account, "id": id, "token": plain})
	return nil
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request, c call) error {
	op := change.Op{Kind: change.RevokeToken, TokenID: r.PathValue("id")}
	e, err := s.log.Commit(string(c.actor), r.URL.Query().Get("reason"), op)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"change": viewEntry(e)})
	return nil
}
