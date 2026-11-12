package api

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Captures a person starts (0148): what crosses an AP, taken in the AP's
// own path, on the Wi-Fi side of a network, on its wired side, or both in
// one file; of one client, of a whole network, or of a VLAN. The AP takes
// it and sends it up in pieces as it runs; the manager keeps the pieces as
// files beside its databases and gives the capture out as one pcapng.
//
// A capture is an action of its AP (0104), of kind capture: what was asked
// is its params, and what came of it its detail. Called pcaps here, to keep
// them from the captures of failed attempts (0135).

const (
	pcapKind         = "capture"
	pcapSeconds      = 60       // how long one runs, unless told
	pcapSecondsMost  = 300      // and at the most: an action waits ten minutes for its AP, the sending included
	pcapSnap         = 128      // bytes kept of each packet, unless whole ones are asked for
	pcapBytes        = 20 << 20 // bytes an AP may keep of one, of packets' headers
	pcapBytesWhole   = 50 << 20 // and of whole packets
	pcapPiece        = 512 << 10
	pcapFilesMost    = 16
	pcapsKept        = 60 // captures kept, the latest
	pcapsKeptFor     = 30 * 24 * time.Hour
	pcapListed       = 200
	pcapSidesAllowed = "wireless, wired or both"
)

// pcapAsk is what a capture is asked to take: which sides; of which
// network, by its ID, or which VLAN; on which bands, none for all; of which
// client, none for everyone; whole packets or their first bytes; for how
// long, and how much at the most.
type pcapAsk struct {
	Sides   string   `json:"sides"`
	Network string   `json:"network,omitempty"`
	SSID    string   `json:"ssid,omitempty"`
	VLAN    int      `json:"vlan,omitempty"`
	Bands   []string `json:"bands,omitempty"`
	Client  string   `json:"client,omitempty"`
	Whole   bool     `json:"whole,omitempty"`
	Snap    int      `json:"snap"`
	Seconds int      `json:"seconds"`
	Bytes   int      `json:"bytes"`
}

// pcapFile is one interface's part of a capture, as its AP sent it: which
// of them, the interface, its side, what it is of in words, and how much of
// it the manager holds.
type pcapFile struct {
	N     int    `json:"n"`
	Name  string `json:"name"`
	Side  string `json:"side"`
	Note  string `json:"note,omitempty"`
	Bytes int64  `json:"bytes"`
}

// pcapMeta is what the manager notes beside a capture's files: the files;
// that someone asked for it to stop, which its AP is told when it next
// sends a piece; that its AP was told of it at once, on its line; and how
// long after hearing so the AP began to take it, in ms, as the AP says.
type pcapMeta struct {
	Files  []pcapFile `json:"files"`
	Stop   bool       `json:"stop,omitempty"`
	Told   bool       `json:"told,omitempty"`
	TookMS *int       `json:"took_ms,omitempty"`
}

// WithPcaps says where the captures people start are kept (0148): a
// directory, made when first needed.
func (s *Server) WithPcaps(dir string) *Server {
	s.pcapDir = dir
	return s
}

func (s *Server) pcapPath(id int64, name string) string {
	return filepath.Join(s.pcapDir, strconv.FormatInt(id, 10), name)
}

// pcapMetaOf reads what is noted of a capture; one with nothing noted has
// no files yet.
func (s *Server) pcapMetaOf(id int64) pcapMeta {
	var m pcapMeta
	if b, err := os.ReadFile(s.pcapPath(id, "meta.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (s *Server) pcapMetaPut(id int64, m pcapMeta) error {
	if err := os.MkdirAll(filepath.Dir(s.pcapPath(id, "meta.json")), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(m)
	tmp := s.pcapPath(id, "meta.json.new")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.pcapPath(id, "meta.json"))
}

// pcapStart asks an AP for a capture (0148): POST /v1/aps/{ap}/pcaps
// {sides, network | vlan, bands?, client?, whole?, seconds?}. Packets are
// an admin's (0177). One runs on an AP at a time: the AP says so of a
// second.
func (s *Server) pcapStart(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || !may(c, change.Locations, t, id, access.View) {
		return errNotFound
	}
	if !may(c, change.Locations, t, id, access.Capture) {
		return &apiError{http.StatusForbidden, "a capture takes packets, which needs the capture right on the AP: an admin's (0177)"}
	}
	if s.pcapDir == "" {
		return &apiError{http.StatusServiceUnavailable, "this manager has nowhere to keep captures"}
	}
	var body struct {
		Sides   string   `json:"sides"`
		Network string   `json:"network"`
		VLAN    int      `json:"vlan"`
		Bands   []string `json:"bands"`
		Client  string   `json:"client"`
		Whole   bool     `json:"whole"`
		Seconds int      `json:"seconds"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	ask := pcapAsk{Sides: body.Sides, Network: body.Network, VLAN: body.VLAN, Client: strings.ToLower(body.Client), Whole: body.Whole,
		Snap: pcapSnap, Seconds: body.Seconds, Bytes: pcapBytes}
	if ask.Sides != "wireless" && ask.Sides != "wired" && ask.Sides != "both" {
		return badRequest("sides: %s", pcapSidesAllowed)
	}
	if (ask.Network == "") == (ask.VLAN == 0) {
		return badRequest("a capture is of a network, by its ID, or of a VLAN: one of the two")
	}
	if ask.VLAN < 0 || ask.VLAN > 4094 {
		return badRequest("vlan: from 1 to 4094")
	}
	if ask.Client != "" && !macRE.MatchString(ask.Client) {
		return badRequest("client: a MAC, such as aa:bb:cc:00:11:22")
	}
	if ask.Network != "" {
		res, err := s.compose(c.state, id, s.reveal)
		if err != nil {
			return err
		}
		nets, _ := res.Doc["network"].(map[string]any)
		n, ok := nets[ask.Network].(map[string]any)
		if !ok {
			return badRequest("network: one of the networks this AP offers")
		}
		ask.SSID, _ = n["ssid"].(string)
	}
	for _, b := range body.Bands {
		if !bands[b] {
			return badRequest("bands: 2g, 5g or 6g, none for all of them")
		}
		if !slices.Contains(ask.Bands, b) {
			ask.Bands = append(ask.Bands, b)
		}
	}
	slices.Sort(ask.Bands)
	if ask.Seconds == 0 {
		ask.Seconds = pcapSeconds
	}
	if ask.Seconds < 5 || ask.Seconds > pcapSecondsMost {
		return badRequest("seconds: from 5 to %d", pcapSecondsMost)
	}
	if ask.Whole {
		ask.Snap, ask.Bytes = 0, pcapBytesWhole
	}
	params, err := json.Marshal(ask)
	if err != nil {
		return err
	}
	s.pcapPrune()
	a, err := s.conds.AddActionWith(id, pcapKind, ask.Client, string(c.actor), params)
	if err != nil {
		return err
	}
	// One that waits already stands for this asking only where it asks the
	// same.
	if string(a.Params) != string(params) {
		return &apiError{http.StatusConflict, "a capture is asked of this AP already, and waits for it"}
	}
	// Its AP is told at once, on the line it holds open (0145), and begins
	// within the second: someone who asks for a capture is about to do what
	// they want captured. An AP that holds no line takes it up at its poll.
	told := false
	if a.State == "pending" {
		b, _ := json.Marshal(liveSay{Now: msOf(time.Now()), Capture: &liveCapture{ID: a.ID, Params: params}})
		if told = s.live.tellOne(id, append(b, '\n')); told {
			s.pcapMu.Lock()
			m := s.pcapMetaOf(a.ID)
			m.Told = true
			_ = s.pcapMetaPut(a.ID, m)
			s.pcapMu.Unlock()
		}
	}
	slog.Info("capture asked", "ap", id, "id", a.ID, "sides", ask.Sides, "network", ask.Network, "vlan", ask.VLAN, "client", ask.Client, "whole", ask.Whole, "seconds", ask.Seconds, "told", told, "by", c.actor)
	writeJSON(w, http.StatusOK, map[string]any{"capture": s.pcapView(c, a)})
	return nil
}

// pcapSaid is a capture as a list has it: the action it is, its AP by name
// and place, what was asked, and the files held of it.
type pcapSaid struct {
	conditions.Action
	APName string      `json:"ap_name"`
	Place  []placeStep `json:"place"`
	Ask    pcapAsk     `json:"ask"`
	Files  []pcapFile  `json:"files"`
	Bytes  int64       `json:"bytes"`
	Stop   bool        `json:"stopping,omitempty"`
	// Its AP was told of it at once, on its line; and how long after
	// hearing so the AP began to take it, in ms.
	Told   bool `json:"told,omitempty"`
	TookMS *int `json:"took_ms,omitempty"`
}

func (s *Server) pcapView(c call, a conditions.Action) pcapSaid {
	out := pcapSaid{Action: a, Files: []pcapFile{}}
	_ = json.Unmarshal(a.Params, &out.Ask)
	out.Params = nil
	t := c.state.Org.Locations
	if n, ok := t.Node(hierarchy.NodeID(a.AP)); ok {
		out.APName = n.Name
	}
	out.Place = placeOf(t, hierarchy.NodeID(a.AP))
	m := s.pcapMetaOf(a.ID)
	for _, f := range m.Files {
		out.Files = append(out.Files, f)
		out.Bytes += f.Bytes
	}
	out.Stop = m.Stop && (a.State == "pending" || a.State == "running")
	out.Told, out.TookMS = m.Told, m.TookMS
	return out
}

// pcapList is the captures people started, on the APs the caller may view,
// the latest first: GET /v1/pcaps, ?ap= for one AP's, ?client= for those
// of one client.
func (s *Server) pcapList(w http.ResponseWriter, r *http.Request, c call) error {
	list, err := s.conds.ActionsOfKind(pcapKind, pcapListed)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	ap, client := r.URL.Query().Get("ap"), strings.ToLower(r.URL.Query().Get("client"))
	out := []pcapSaid{}
	for _, a := range list {
		if !may(c, change.Locations, t, hierarchy.NodeID(a.AP), access.Capture) || (ap != "" && a.AP != ap) || (client != "" && a.Target != client) {
			continue
		}
		out = append(out, s.pcapView(c, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"captures": out})
	return nil
}

// pcapOf is the capture with that ID, where the caller may view its AP.
func (s *Server) pcapOf(r *http.Request, c call) (conditions.Action, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return conditions.Action{}, errNotFound
	}
	a, ok, err := s.conds.ActionOfKind(id, pcapKind)
	if err != nil {
		return conditions.Action{}, err
	}
	if !ok || !may(c, change.Locations, c.state.Org.Locations, hierarchy.NodeID(a.AP), access.Capture) {
		return conditions.Action{}, errNotFound
	}
	return a, nil
}

// pcapStop has a capture end before its time: POST /v1/pcaps/{id}/stop.
// Its AP is told when it next sends a piece, some seconds on at the most,
// and sends what it has.
func (s *Server) pcapStop(w http.ResponseWriter, r *http.Request, c call) error {
	a, err := s.pcapOf(r, c)
	if err != nil {
		return err
	}
	if a.State != "pending" && a.State != "running" {
		return &apiError{http.StatusConflict, "this capture has ended"}
	}
	s.pcapMu.Lock()
	m := s.pcapMetaOf(a.ID)
	m.Stop = true
	err = s.pcapMetaPut(a.ID, m)
	s.pcapMu.Unlock()
	if err != nil {
		return err
	}
	slog.Info("capture stopped", "ap", a.AP, "id", a.ID, "by", c.actor)
	writeJSON(w, http.StatusOK, map[string]any{"capture": s.pcapView(c, a)})
	return nil
}

// pcapDelete takes a capture away, its files and its record: DELETE
// /v1/pcaps/{id}. One that still runs is stopped first.
func (s *Server) pcapDelete(w http.ResponseWriter, r *http.Request, c call) error {
	a, err := s.pcapOf(r, c)
	if err != nil {
		return err
	}
	if a.State == "pending" || a.State == "running" {
		return &apiError{http.StatusConflict, "this capture still runs: stop it first"}
	}
	s.pcapMu.Lock()
	err = os.RemoveAll(filepath.Dir(s.pcapPath(a.ID, "meta.json")))
	s.pcapMu.Unlock()
	if err != nil {
		return err
	}
	if err := s.conds.DeleteAction(a.ID, pcapKind); err != nil {
		return err
	}
	slog.Info("capture deleted", "ap", a.AP, "id", a.ID, "by", c.actor)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": a.ID})
	return nil
}

// pcapPrune lets go of old captures: past the latest so many, and past so
// many days. Called when one is asked for.
func (s *Server) pcapPrune() {
	list, err := s.conds.ActionsOfKind(pcapKind, 1000)
	if err != nil {
		return
	}
	for i, a := range list {
		if a.State == "pending" || a.State == "running" || (i < pcapsKept && time.Since(a.At) < pcapsKeptFor) {
			continue
		}
		s.pcapMu.Lock()
		_ = os.RemoveAll(filepath.Dir(s.pcapPath(a.ID, "meta.json")))
		s.pcapMu.Unlock()
		_ = s.conds.DeleteAction(a.ID, pcapKind)
	}
}

// pcapPiece takes a piece of a capture from its AP (0148): POST
// /v1/ap/pcaps/{id} {n, name, side, note, offset, data, took?}, the nth
// interface of the capture, what it is, where in its file the piece begins,
// and the piece, in base64, half a megabyte at the most; took is how long
// after hearing of the capture on its line the AP began it, in ms. With no
// data it only asks whether to go on. The answer says how much of the file is held, and
// whether someone asked for the capture to stop. A piece sent again, as
// where the answer to it was lost, is not kept twice; one that leaves a
// gap is refused, and the AP sends from where the manager is.
func (s *Server) pcapPiece(w http.ResponseWriter, r *http.Request, c apCall) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return errNotFound
	}
	if s.pcapDir == "" {
		return &apiError{http.StatusServiceUnavailable, "this manager has nowhere to keep captures"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, pcapPiece/3*4+4096)
	var body struct {
		N      *int   `json:"n"`
		Name   string `json:"name"`
		Side   string `json:"side"`
		Note   string `json:"note"`
		Offset int64  `json:"offset"`
		Data   string `json:"data"`
		// How long after it heard of the capture on its line the AP began
		// to take it, in ms: said with every piece, and kept once.
		Took *int `json:"took"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	a, err := s.conds.OpenAction(c.ap, id)
	if errors.Is(err, conditions.ErrNoAction) || (err == nil && a.Kind != pcapKind) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	var ask pcapAsk
	_ = json.Unmarshal(a.Params, &ask)
	s.pcapMu.Lock()
	defer s.pcapMu.Unlock()
	m := s.pcapMetaOf(id)
	held := int64(0)
	if body.Took != nil && m.TookMS == nil && *body.Took >= 0 && *body.Took <= 60000 {
		m.TookMS = body.Took
		if err := s.pcapMetaPut(id, m); err != nil {
			return err
		}
	}
	if body.N != nil {
		n := *body.N
		data, err := base64.StdEncoding.DecodeString(body.Data)
		if err != nil {
			return badRequest("a piece of a capture: its data is in base64")
		}
		if n < 0 || n >= pcapFilesMost || !shortText(body.Name, 32) || body.Name == "" || (body.Side != "wireless" && body.Side != "wired") ||
			len(body.Note) > 80 || !printable(body.Note) || body.Offset < 0 || len(data) > pcapPiece {
			return badRequest("a piece of a capture: n, from 0 to %d; the interface's name; its side, wireless or wired; a note of it; where it begins; and at most half a megabyte of it", pcapFilesMost-1)
		}
		at := slices.IndexFunc(m.Files, func(f pcapFile) bool { return f.N == n })
		if at < 0 {
			m.Files = append(m.Files, pcapFile{N: n, Name: body.Name, Side: body.Side, Note: body.Note})
			at = len(m.Files) - 1
		}
		f := &m.Files[at]
		total := int64(0)
		for _, x := range m.Files {
			total += x.Bytes
		}
		switch {
		case body.Offset+int64(len(data)) <= f.Bytes:
			// Sent before.
		case body.Offset != f.Bytes:
			writeJSON(w, http.StatusConflict, map[string]any{"error": "a piece of a capture: it does not begin where the last ended", "bytes": f.Bytes, "stop": m.Stop})
			return nil
		case total+int64(len(data)) > int64(ask.Bytes)+int64(ask.Bytes)/4+1<<20:
			return &apiError{http.StatusRequestEntityTooLarge, "a capture: more than it was asked to hold"}
		default:
			if err := os.MkdirAll(filepath.Dir(s.pcapPath(id, "x")), 0o700); err != nil {
				return err
			}
			file, err := os.OpenFile(s.pcapPath(id, strconv.Itoa(n)+".pcap"), os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			_, err = file.WriteAt(data, f.Bytes)
			if cerr := file.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			f.Bytes += int64(len(data))
			if err := s.pcapMetaPut(id, m); err != nil {
				return err
			}
		}
		held = f.Bytes
	}
	writeJSON(w, http.StatusOK, map[string]any{"bytes": held, "stop": m.Stop})
	return nil
}

// pcapDetail holds what an AP says came of a capture to a small object:
// how many packets it took, why it ended, and what it could not do.
func pcapDetail(raw json.RawMessage) (json.RawMessage, error) {
	var d struct {
		Packets int      `json:"packets"`
		Why     string   `json:"why"`
		Missed  []string `json:"missed"`
	}
	if len(raw) > 4096 || json.Unmarshal(raw, &d) != nil || d.Packets < 0 || len(d.Why) > 120 || !printable(d.Why) || len(d.Missed) > pcapFilesMost {
		return nil, badRequest("detail: of a capture, how many packets it took, why it ended, and the interfaces it could not take")
	}
	for i, m := range d.Missed {
		if len(m) > 120 || !printable(m) {
			return nil, badRequest("detail: of a capture, each interface it could not take in at most 120 printable characters")
		}
		d.Missed[i] = m
	}
	return json.Marshal(d)
}

// pcapRead gives a capture as one file (0148): GET /v1/pcaps/{id}/file, a
// pcapng with an interface for each the AP took, named for what it is, and
// their packets together by when they crossed. One still running gives
// what has come so far. Each reading is logged with who.
func (s *Server) pcapRead(w http.ResponseWriter, r *http.Request, c call) error {
	a, err := s.pcapOf(r, c)
	if err != nil {
		return err
	}
	s.pcapMu.Lock()
	m := s.pcapMetaOf(a.ID)
	s.pcapMu.Unlock()
	if len(m.Files) == 0 {
		return &apiError{http.StatusNotFound, "nothing has come of this capture"}
	}
	name := a.AP
	if n, ok := c.state.Org.Locations.Node(hierarchy.NodeID(a.AP)); ok && fileName(n.Name) != "" {
		name = fileName(n.Name)
	}
	var ask pcapAsk
	_ = json.Unmarshal(a.Params, &ask)
	of := fileName(ask.Network)
	if ask.VLAN != 0 {
		of = fmt.Sprintf("vlan%d", ask.VLAN)
	}
	if ask.Client != "" {
		of = strings.ReplaceAll(ask.Client, ":", "")
	}
	name = fmt.Sprintf("aeolus-%s-%s-%s.pcapng", name, of, a.At.UTC().Format("20060102-150405Z"))
	var srcs []pcapSource
	for _, f := range m.Files {
		file, err := os.Open(s.pcapPath(a.ID, strconv.Itoa(f.N)+".pcap"))
		if err != nil {
			continue
		}
		defer file.Close()
		what := f.Side
		if f.Note != "" {
			what += ": " + f.Note
		}
		srcs = append(srcs, pcapSource{r: bufio.NewReaderSize(file, 256<<10), name: f.Name, what: what})
	}
	slog.Info("capture read", "ap", a.AP, "id", a.ID, "by", c.actor)
	w.Header().Set("Content-Type", "application/x-pcapng")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	out := bufio.NewWriterSize(w, 256<<10)
	if err := mergePcaps(out, srcs); err != nil {
		slog.Warn("capture read: writing it", "err", err)
	}
	return out.Flush()
}

// pcapSource is one interface's packets as tcpdump wrote them: a pcap
// file, read from its start.
type pcapSource struct {
	r    io.Reader
	name string
	what string

	order  binary.ByteOrder
	nanos  bool
	link   uint32
	snap   uint32
	ok     bool
	ts     uint64 // the next packet's time, in the file's own units
	caplen uint32
	orig   uint32
	data   []byte
}

// next reads the source's next packet; ok is false once it has no more
// whole ones, as at the end of a file still being written.
func (p *pcapSource) next() {
	var h [16]byte
	p.ok = false
	if _, err := io.ReadFull(p.r, h[:]); err != nil {
		return
	}
	sec, frac := p.order.Uint32(h[0:]), p.order.Uint32(h[4:])
	p.caplen, p.orig = p.order.Uint32(h[8:]), p.order.Uint32(h[12:])
	if p.caplen > 262144 {
		return
	}
	if cap(p.data) < int(p.caplen) {
		p.data = make([]byte, p.caplen)
	}
	p.data = p.data[:p.caplen]
	if _, err := io.ReadFull(p.r, p.data); err != nil {
		return
	}
	unit := uint64(1e6)
	if p.nanos {
		unit = 1e9
	}
	p.ts = uint64(sec)*unit + uint64(frac)
	p.ok = true
}

// mergePcaps writes several pcap files as one pcapng: an interface for
// each, with its name and what it is, and their packets in one line by
// time. A file that is no pcap is left out.
func mergePcaps(w io.Writer, srcs []pcapSource) error {
	le := binary.LittleEndian
	block := func(kind uint32, body []byte) error {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		head := le.AppendUint32(le.AppendUint32(nil, kind), uint32(12+len(body)))
		if _, err := w.Write(head); err != nil {
			return err
		}
		if _, err := w.Write(body); err != nil {
			return err
		}
		_, err := w.Write(head[4:])
		return err
	}
	option := func(b []byte, code uint16, v []byte) []byte {
		b = le.AppendUint16(le.AppendUint16(b, code), uint16(len(v)))
		b = append(b, v...)
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		return b
	}
	shb := le.AppendUint32(nil, pcapOrder)
	shb = append(shb, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)
	shb = option(shb, 4, []byte("Aeolus"))
	shb = append(shb, 0, 0, 0, 0)
	if err := block(pcapSection, shb); err != nil {
		return err
	}
	var live []*pcapSource
	for i := range srcs {
		p := &srcs[i]
		var h [24]byte
		if _, err := io.ReadFull(p.r, h[:]); err != nil {
			continue
		}
		switch binary.LittleEndian.Uint32(h[:]) {
		case 0xa1b2c3d4:
			p.order = binary.LittleEndian
		case 0xa1b23c4d:
			p.order, p.nanos = binary.LittleEndian, true
		case 0xd4c3b2a1:
			p.order = binary.BigEndian
		case 0x4d3cb2a1:
			p.order, p.nanos = binary.BigEndian, true
		default:
			continue
		}
		p.snap, p.link = p.order.Uint32(h[16:]), p.order.Uint32(h[20:])&0xffff
		idb := le.AppendUint16(nil, uint16(p.link))
		idb = le.AppendUint32(append(idb, 0, 0), p.snap)
		idb = option(idb, 2, []byte(p.name))
		if p.what != "" {
			idb = option(idb, 3, []byte(p.what))
		}
		res := byte(6)
		if p.nanos {
			res = 9
		}
		idb = option(idb, 9, []byte{res})
		idb = append(idb, 0, 0, 0, 0)
		if err := block(pcapInterface, idb); err != nil {
			return err
		}
		p.next()
		live = append(live, p)
	}
	// Each in its own units of time; told apart in nanoseconds.
	when := func(p *pcapSource) uint64 {
		if p.nanos {
			return p.ts
		}
		return p.ts * 1000
	}
	for {
		first := -1
		for i, p := range live {
			if p.ok && (first < 0 || when(p) < when(live[first])) {
				first = i
			}
		}
		if first < 0 {
			return nil
		}
		p := live[first]
		epb := le.AppendUint32(nil, uint32(first))
		epb = le.AppendUint32(epb, uint32(p.ts>>32))
		epb = le.AppendUint32(epb, uint32(p.ts))
		epb = le.AppendUint32(epb, p.caplen)
		epb = le.AppendUint32(epb, p.orig)
		epb = append(epb, p.data...)
		if err := block(pcapFrame, epb); err != nil {
			return err
		}
		p.next()
	}
}
