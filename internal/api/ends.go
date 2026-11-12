package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// EndChanges ends each change that ends itself once its until has come
// (0345), in the manager's own name, now and every so often until ctx is
// done. A manager that was down ends what came due meanwhile when it is
// back.
func (s *Server) EndChanges(ctx context.Context, every time.Duration) {
	look := func() {
		for _, seq := range s.log.Due(time.Now()) {
			e, left, err := s.log.End(change.SystemActor, "", seq)
			if err != nil {
				slog.Warn("ends: change not ended", "change", seq, "err", err)
				continue
			}
			slog.Info("ends: change ended at its until", "change", seq, "end", e.Seq, "put_back", len(e.Op.Ops), "left", len(left))
		}
	}
	look()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			look()
		}
	}
}

// endStep is a step of an end as it is shown: where, and not the values,
// which may be secrets.
type endStep struct {
	Tree   change.TreeName  `json:"tree"`
	Node   hierarchy.NodeID `json:"node"`
	Path   hierarchy.Path   `json:"path,omitempty"`
	Folder bool             `json:"folder,omitempty"`
}

// endsList lists the changes that end themselves, not ended yet, the
// soonest first. As the change log, it needs viewer at the Org root of
// either tree.
func (s *Server) endsList(w http.ResponseWriter, r *http.Request, c call) error {
	root := c.state.Org.Locations.Root()
	if !may(c, change.Locations, c.state.Org.Locations, root, access.View) &&
		!may(c, change.Services, c.state.Org.Services, root, access.View) {
		return &apiError{http.StatusForbidden, "reading what ends needs viewer at the Org root"}
	}
	type endView struct {
		Seq    int64     `json:"seq"`
		At     time.Time `json:"at"`
		Actor  string    `json:"actor"`
		Reason string    `json:"reason,omitempty"`
		Until  time.Time `json:"until"`
		Steps  []endStep `json:"steps"`
	}
	ends := s.log.Ends()
	out := make([]endView, 0, len(ends))
	for _, p := range ends {
		steps := make([]endStep, len(p.Steps))
		for i, st := range p.Steps {
			steps[i] = endStep{Tree: st.Tree, Node: st.Node, Path: st.Path, Folder: st.Folder}
		}
		out = append(out, endView{Seq: p.Seq, At: p.At, Actor: p.Actor, Reason: p.Reason, Until: p.Until, Steps: steps})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ends": out})
	return nil
}

// endOne ends a change that ends itself now, or, with until, moves its end
// there: {until, reason, tree, node, values}, all optional. With values,
// the move sets those fields of node as part of the change, checked and
// sealed as a set of them is: a test window's maintenance.until, moved
// with it.
func (s *Server) endOne(w http.ResponseWriter, r *http.Request, c call) error {
	seq, err := strconv.ParseInt(r.PathValue("seq"), 10, 64)
	if err != nil || seq <= 0 {
		return badRequest("the change is its sequence number")
	}
	var req struct {
		Until  *time.Time                         `json:"until"`
		Reason string                             `json:"reason"`
		Tree   change.TreeName                    `json:"tree"`
		Node   hierarchy.NodeID                   `json:"node"`
		Values map[hierarchy.Path]json.RawMessage `json:"values"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &req); err != nil {
			return err
		}
	}
	if len(req.Values) > 0 && req.Until == nil {
		return badRequest("values are set with a new until")
	}
	if req.Until != nil {
		var set *change.Op
		if len(req.Values) > 0 {
			op, err := s.prepare(change.Op{Kind: change.Set, Tree: req.Tree, Node: req.Node, Values: req.Values})
			if err != nil {
				return err
			}
			set = &op
		}
		e, err := s.log.MoveEnd(string(c.actor), req.Reason, seq, *req.Until, set)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]any{"change": viewEntry(e)})
		return nil
	}
	e, left, err := s.log.End(string(c.actor), req.Reason, seq)
	if err != nil {
		return err
	}
	after := s.log.Snapshot()
	reversioned := s.reversioned(after, e.Seq)
	writeJSON(w, http.StatusOK, map[string]any{
		"change":      viewEntry(e),
		"left":        left,
		"reversioned": reversioned,
	})
	return nil
}
