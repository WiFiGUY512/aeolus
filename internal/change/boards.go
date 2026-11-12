package change

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/library"
)

// Settings for one kind of AP (0092): a Locations field under
// boards.<board>., such as boards.arista,c-360.ports.eth1.mode, is the plain
// field (ports.eth1.mode) for the APs of that board alone, so a folder can
// set the ports of each kind of AP it holds without one kind's settings
// reaching another's ports of the same name. It inherits like any field.
// An AP of the board takes it in place of the plain one set on the same
// node or above; a plain one set closer to the AP, or locked above, stays.
// Ports only, for now.

// BoardsPrefix starts the Locations fields for one kind of AP.
const BoardsPrefix = "boards."

// BoardPath is p for the APs of one board.
func BoardPath(board string, p hierarchy.Path) hierarchy.Path {
	return hierarchy.Path(BoardsPrefix + board + "." + string(p))
}

// currentBoards is op with the boards it names by the names OpenWrt gives
// them now (0174): a renamed board's templates.<board> and boards.<board>.
// fields, and a template's boards. So a change made under a board's old
// name, before its rename or after, lands where one under the new name
// does, and replaying the log puts what was set under old names under the
// new. Op's own maps and slices are left as they are: the log keeps what
// was said. Where one change sets a field under both names, the new name's
// value is the one kept.
func currentBoards(op Op) Op {
	op.Path = currentBoardPath(op.Path)
	if op.Values != nil {
		vals := make(map[hierarchy.Path]json.RawMessage, len(op.Values))
		var renamed []hierarchy.Path
		for p, v := range op.Values {
			if currentBoardPath(p) != p {
				renamed = append(renamed, p)
				continue
			}
			vals[p] = v
		}
		sort.Slice(renamed, func(i, j int) bool { return renamed[i] < renamed[j] })
		for _, p := range renamed {
			if q := currentBoardPath(p); !hasPath(vals, q) {
				vals[q] = op.Values[p]
			}
		}
		op.Values = vals
	}
	if op.Paths != nil {
		paths := make([]hierarchy.Path, 0, len(op.Paths))
		for _, p := range op.Paths {
			if q := currentBoardPath(p); !slices.Contains(paths, q) {
				paths = append(paths, q)
			}
		}
		op.Paths = paths
	}
	op.Boards = library.Boards(op.Boards)
	return op
}

func hasPath(m map[hierarchy.Path]json.RawMessage, p hierarchy.Path) bool {
	_, ok := m[p]
	return ok
}

// currentBoardPath is p with a renamed board in it by its new name, if p is
// a templates.<board> or boards.<board>. field; any other p as it is.
func currentBoardPath(p hierarchy.Path) hierarchy.Path {
	for _, prefix := range []string{TemplatesPrefix, BoardsPrefix} {
		rest, ok := strings.CutPrefix(string(p), prefix)
		if !ok {
			continue
		}
		board, field, sub := strings.Cut(rest, ".")
		now := library.Board(board)
		if now == board {
			return p
		}
		if sub {
			return hierarchy.Path(prefix + now + "." + field)
		}
		return hierarchy.Path(prefix + now)
	}
	return p
}

// foldBoards puts the board's fields in place of the plain ones they
// replace, and takes every board's out of the AP's settings: none is sent
// to an AP as it is. chain is the AP and the nodes it inherits from,
// nearest first.
func foldBoards(cfg *hierarchy.APConfig, board string, chain []hierarchy.NodeID) {
	prefix := BoardsPrefix + board + "."
	var mine []hierarchy.Path
	for p := range cfg.Location {
		if !strings.HasPrefix(string(p), BoardsPrefix) {
			continue
		}
		if board != "" && strings.HasPrefix(string(p), prefix) {
			mine = append(mine, p)
			continue
		}
		delete(cfg.Location, p)
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i] < mine[j] })
	for _, p := range mine {
		r := cfg.Location[p]
		delete(cfg.Location, p)
		plain := hierarchy.Path(strings.TrimPrefix(string(p), prefix))
		if cur, set := cfg.Location[plain]; set {
			if cur.Origin == hierarchy.OriginLocked && cur.From != r.From {
				continue // a lock above holds
			}
			if i := slices.Index(chain, cur.From); i >= 0 && i < slices.Index(chain, r.From) {
				continue // set closer to the AP
			}
		}
		cfg.Location[plain] = r
	}
}
