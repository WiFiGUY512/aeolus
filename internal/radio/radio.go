// Package radio knows what Wi-Fi radios can do: the widths each band allows,
// the htmodes that express them, and the 5 GHz channels that can carry each
// width. The render check, the config checks and the hardware offered on a
// folder all read it, so they always agree.
package radio

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Widths are the channel widths, in MHz, the schema allows on each band.
var Widths = map[string][]int{
	"2g": {20, 40},
	"5g": {20, 40, 80, 160},
	"6g": {20, 40, 80, 160, 320},
}

// Bands in the order people read them.
var Bands = []string{"2g", "5g", "6g"}

// ModeWidth reads the width of an htmode such as HT40, VHT160 or HE80.
// VHT80+80 and NOHT are not widths Aeolus asks for.
func ModeWidth(mode string) (int, bool) {
	for _, fam := range []string{"EHT", "HE", "VHT", "HT"} {
		rest, ok := strings.CutPrefix(mode, fam)
		if !ok {
			continue
		}
		w, err := strconv.Atoi(rest)
		return w, err == nil && w > 0
	}
	return 0, false
}

// Can lists the widths a radio can use, from the htmodes it reported.
func Can(htmodes []string) map[int]bool {
	out := map[int]bool{}
	for _, m := range htmodes {
		if w, ok := ModeWidth(m); ok {
			out[w] = true
		}
	}
	return out
}

// A Grant is what a model's regulatory grant covers, where its radios would
// do more (0302): a radio's self-managed rules are not its grant. Most is
// the widest on each band, in MHz; Off, the channels of each band the
// grant leaves out.
type Grant struct {
	Most map[string]int
	Off  map[string][]int
}

// Granted holds each model to its grant, keyed by the board name the AP
// reports. A model or band not listed is held only to what its radio
// reports. Add a row as a grant is read, saying where.
//
// The Arista rows are from their FCC grants (grantee TOR), read by the
// porting session from fccid.io on 2026-11-04 (Arista APs
// build/compliance/fcc-grants.md). None covers 160 MHz on 5 GHz. None
// covers U-NII-4 (169–177) either, which the radios' own board data
// already stops short of (the W-318's by port 0077), so it is no row.
var Granted = map[string]Grant{
	// TOR-C360 (2022-03-02; the C2PC of 2022-05-17 added U-NII-2A and 2C).
	// Its QCN9074's rules would allow 160 MHz on 5 GHz (porting, B103).
	"arista,c-360": {Most: map[string]int{"2g": 40, "5g": 80, "6g": 160}},
	"arista,w-318": {Most: map[string]int{"2g": 40, "5g": 80, "6g": 160}}, // TOR-W318
	"arista,c-200": {Most: map[string]int{"2g": 40, "5g": 80}},            // TOR-C200
	"arista,c-230": {Most: map[string]int{"2g": 40, "5g": 80}},            // TOR-C230: 80+80, no 160 in one
	// TOR-W118: its U-NII-2C is 5500–5700 MHz, so channel 144 (5720) is out.
	"arista,w-118": {Most: map[string]int{"2g": 40, "5g": 80}, Off: map[string][]int{"5g": {144}}},
	// Not ported yet: the board names are as the port names the others.
	"arista,c-130": {Most: map[string]int{"2g": 40, "5g": 80}},                                     // TOR-C130: 80+80
	"arista,c-250": {Most: map[string]int{"2g": 40, "5g": 80}, Off: map[string][]int{"5g": {144}}}, // TOR-C250: U-NII-2C 5500–5700
	// The C-260 has no grant under TOR: its FCC ID is on its label, not yet
	// read. Held to 80 MHz on 5 GHz, as every grant read is, until it is.
	"arista,c-260": {Most: map[string]int{"5g": 80}},
}

// GrantedMost says the widest a board is granted on a band, where it is
// held to less than its radio reports.
func GrantedMost(board, band string) (int, bool) {
	w, ok := Granted[board].Most[band]
	return w, ok
}

// GrantedOff lists the channels of a band a board's grant leaves out.
func GrantedOff(board, band string) []int {
	return Granted[board].Off[band]
}

// OffIn says which channel a board's grant leaves out is in the block a
// channel at a width uses: 0 where none is.
func OffIn(board, band string, channel, width int) int {
	off := GrantedOff(board, band)
	if len(off) == 0 {
		return 0
	}
	lo, hi := channel, channel
	if gs, ok := groups(band, width); ok {
		for _, g := range gs {
			if channel >= g[0] && channel <= g[1] {
				lo, hi = g[0], g[1]
				break
			}
		}
	}
	for _, c := range off {
		if c >= lo && c <= hi {
			return c
		}
	}
	return 0
}

// CanOn lists the widths a radio of a board can use on a band: what its
// htmodes say, and no wider than the model's grant.
func CanOn(board, band string, htmodes []string) map[int]bool {
	can := Can(htmodes)
	if most, ok := GrantedMost(board, band); ok {
		for w := range can {
			if w > most {
				delete(can, w)
			}
		}
	}
	return can
}

// groups5g are the 5 GHz channels that can be joined for each width, by the
// lowest and highest 20 MHz channel in the group, the US's: the upper ones,
// 165–177, among them (0232). Channel compatibility, on unless turned off,
// keeps every radio out of those, so one is asked for only where a person
// turned it off.
var groups5g = map[int][][2]int{
	40:  {{36, 40}, {44, 48}, {52, 56}, {60, 64}, {100, 104}, {108, 112}, {116, 120}, {124, 128}, {132, 136}, {140, 144}, {149, 153}, {157, 161}, {165, 169}, {173, 177}},
	80:  {{36, 48}, {52, 64}, {100, 112}, {116, 128}, {132, 144}, {149, 161}, {165, 177}},
	160: {{36, 64}, {100, 128}},
}

// groups6g are the 6 GHz channels joined at each width, 1–233 (5925–7125
// MHz), as in the US (0087): a 40 MHz block every 8, 80 every 16, 160
// every 32; 320 MHz blocks come in two families, 1–61 and 33–93 and so on,
// that overlap by 160 MHz.
var groups6g = func() map[int][][2]int {
	g := map[int][][2]int{}
	for _, w := range []struct{ width, span, last int }{{40, 8, 229}, {80, 16, 221}, {160, 32, 221}} {
		for lo := 1; lo+w.span-4 <= w.last; lo += w.span {
			g[w.width] = append(g[w.width], [2]int{lo, lo + w.span - 4})
		}
	}
	for _, lo := range []int{1, 33, 65, 97, 129, 161} {
		g[320] = append(g[320], [2]int{lo, lo + 60})
	}
	return g
}()

// Channels5 are 5 GHz's 20 MHz channels, 36–177, the upper ones too (0232).
var Channels5 = []int{36, 40, 44, 48, 52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 144, 149, 153, 157, 161, 165, 169, 173, 177}

// Upper5 says whether a 5 GHz channel is one of the upper ones, 165–177,
// which channel compatibility keeps every radio out of (0232).
func Upper5(c int) bool { return c >= 165 }

// Compatible5 is the set without the upper channels (0232).
func Compatible5(set []int) []int {
	var out []int
	for _, c := range set {
		if !Upper5(c) {
			out = append(out, c)
		}
	}
	return out
}

// Channels6 are 6 GHz's 20 MHz channels, 1–233.
var Channels6 = func() []int {
	var out []int
	for c := 1; c <= 233; c += 4 {
		out = append(out, c)
	}
	return out
}()

// PSC says whether a 6 GHz channel is a preferred scanning channel: 5, 21,
// 37 and every 16th to 229. A client looks for 6 GHz networks on these by
// itself; anywhere else, only where a neighbour report sends it (0087).
func PSC(channel int) bool {
	return channel >= 5 && channel <= 229 && (channel-5)%16 == 0
}

// PSCOnly keeps the preferred scanning channels of a list.
func PSCOnly(chans []int) []int {
	var out []int
	for _, c := range chans {
		if PSC(c) {
			out = append(out, c)
		}
	}
	return out
}

// groups are the blocks a band's channels are joined in at a width: 5 GHz's
// and 6 GHz's; none elsewhere, or at 20 MHz.
func groups(band string, width int) ([][2]int, bool) {
	switch band {
	case "5g":
		g, ok := groups5g[width]
		return g, ok
	case "6g":
		g, ok := groups6g[width]
		return g, ok
	}
	return nil, false
}

// Edges are the lowest and highest frequencies, in MHz, of the block a
// channel takes up at a width on 5 or 6 GHz (0327); its own 20 MHz at 20
// MHz, or where it is in no block.
func Edges(band string, channel, width int) (int, int) {
	lo, hi := channel, channel
	if g, ok := groups(band, width); ok && width > 20 {
		for _, b := range g {
			if channel >= b[0] && channel <= b[1] {
				lo, hi = b[0], b[1]
				break
			}
		}
	}
	mhz := func(c int) int {
		switch {
		case band == "5g":
			return 5000 + 5*c
		case c == 2:
			return 5935
		}
		return 5950 + 5*c
	}
	return mhz(lo) - 10, mhz(hi) + 10
}

// Fits says whether a channel can carry a width on a band, and if not, why.
// 5 and 6 GHz are checked; channel 0 means automatic, which always fits.
func Fits(band string, channel, width int) (bool, string) {
	if (band != "5g" && band != "6g") || channel == 0 || width <= 20 {
		return true, ""
	}
	groups, ok := groups(band, width)
	if !ok {
		return false, fmt.Sprintf("a %d MHz width is not possible on %s", width, bandName(band))
	}
	var names []string
	for _, g := range groups {
		if channel >= g[0] && channel <= g[1] {
			return true, ""
		}
		names = append(names, fmt.Sprintf("%d–%d", g[0], g[1]))
	}
	if band == "5g" && width == 160 {
		return false, fmt.Sprintf("channel %d cannot use a 160 MHz width; that needs a channel from %s", channel, strings.Join(names, " or "))
	}
	if band == "6g" {
		return false, fmt.Sprintf("channel %d cannot use a %d MHz width on 6 GHz", channel, width)
	}
	return false, fmt.Sprintf("channel %d cannot use a %d MHz width", channel, width)
}

func bandName(band string) string {
	return map[string]string{"2g": "2.4 GHz", "5g": "5 GHz", "6g": "6 GHz"}[band]
}

// Radar says whether a radio on a channel, at a width, uses any of the 5 GHz
// channels shared with radar in the US and EU (DFS), 52–144. Such a radio
// listens for radar before it transmits and moves off if it hears any (0045).
// On channel 0, automatic, it says whether that holds whatever the radio
// picks: every block that can carry the width includes radar channels.
func Radar(band string, channel, width int) bool {
	if band != "5g" {
		return false
	}
	groups := groups5g[width]
	if channel == 0 {
		if len(groups) == 0 {
			return false // 20 MHz: it may pick a channel without radar
		}
		for _, g := range groups {
			if !radar(g[0], g[1]) {
				return false
			}
		}
		return true
	}
	for _, g := range groups {
		if channel >= g[0] && channel <= g[1] {
			return radar(g[0], g[1])
		}
	}
	return radar(channel, channel)
}

func radar(lo, hi int) bool { return lo <= 144 && hi >= 52 }

// The ways a 5 GHz radio uses the DFS channels (0328).
const (
	// DFSAvoid keeps an automatic channel off them (0071).
	DFSAvoid = "avoid"
	// DFSLook starts the radio outside them, and radio resource management
	// moves it to one that is clearly the best once it has looked (0120).
	DFSLook = "look"
	// DFSPick lets hostapd pick one as the radio starts.
	DFSPick = "pick"
)

// Narrow is the width a 5 GHz radio set to 160 MHz starts at where DFS is
// allowed (0329): 80 MHz, outside DFS, for RRM to widen it.
const Narrow = 80

// StartWidth is the width a 5 GHz radio set to width starts at (0329).
// Every 160 MHz block on 5 GHz holds DFS channels, so a radio started at
// 160 MHz waits a minute for its radar check at every start. Where DFS is
// allowed (setting allow, as unset, or look-first), the channel is
// automatic, radio resource management moves radios, and the set (all of
// 5 GHz's where it has none) has a whole 80 MHz block outside DFS, the
// radio starts at 80 MHz there, serving at once, and RRM widens it to 160
// MHz once it has looked, in the window for moves. Elsewhere, width.
func StartWidth(setting string, width int, auto, moves bool, set []int) int {
	if width != 160 || setting == DFSAvoid || !auto || !moves {
		return width
	}
	if len(set) == 0 {
		set = Channels5
	}
	for _, c := range Usable("5g", set, Narrow, false, false, false) {
		if !Radar("5g", c, Narrow) {
			return Narrow
		}
	}
	return width
}

// DFSWay is how a 5 GHz radio uses the DFS channels (0328), by
// radio.5g.dfs: avoid avoids them; allow, the default, as unset, and
// look-first, its name from before 0328, look where they can and pick
// where they cannot. They look where the channel is automatic, radio
// resource management moves radios, and a whole block outside DFS at the
// radio's width is among its channels, the primaries it may use (all of
// 5 GHz's where it has none); elsewhere nothing would look, or the radio
// has nowhere outside DFS to start, as at 160 MHz, and they pick. A
// channel set by hand is used as set: it picks nothing, and is pick.
func DFSWay(setting string, auto, moves bool, channels []int, width int) string {
	if setting == DFSAvoid {
		return DFSAvoid
	}
	if !auto || !moves {
		return DFSPick
	}
	if len(channels) == 0 {
		channels = Channels5
	}
	for _, c := range channels {
		if ok, _ := Fits("5g", c, width); ok && !Radar("5g", c, width) {
			return DFSLook
		}
	}
	return DFSPick
}

// Whole keeps, of a set of channels an automatic channel may be (0075), the
// ones a radio at width can use: on 5 and 6 GHz at 40 MHz or more, those of
// the blocks wholly in the set, as hostapd checks only a block's primary
// channel against its list. Elsewhere, the set itself. With radar, blocks
// shared with radar are dropped too. Sorted, without repeats.
func Whole(band string, set []int, width int, avoidRadar bool) []int {
	in := map[int]bool{}
	for _, c := range set {
		in[c] = true
	}
	var out []int
	blocks, _ := groups(band, width)
	if len(blocks) == 0 || width <= 20 {
		for c := range in {
			if !avoidRadar || band != "5g" || !radar(c, c) {
				out = append(out, c)
			}
		}
		slices.Sort(out)
		return out
	}
	for _, g := range blocks {
		if avoidRadar && band == "5g" && radar(g[0], g[1]) {
			continue
		}
		all := true
		for c := g[0]; c <= g[1]; c += 4 {
			all = all && in[c]
		}
		if all {
			for c := g[0]; c <= g[1]; c += 4 {
				out = append(out, c)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out) // 6 GHz's 320 MHz blocks overlap
}

// SixGHzEncryption is the UCI encryption a network's security takes on
// 6 GHz, and whether it may be there at all (0086). 6 GHz takes WPA3 and
// OWE only: a network in WPA2/WPA3 transition is WPA3 alone there, and one
// that is WPA2 or open is not offered there.
func SixGHzEncryption(security string) (string, bool) {
	switch security {
	case "wpa3-sae", "wpa2-wpa3":
		return "sae", true
	case "owe":
		return "owe", true
	case "wpa3-enterprise", "wpa2-wpa3-enterprise":
		return "wpa3", true // WPA3-Enterprise alone (0098)
	}
	return "", false
}

// TransitionDisable is the Transition Disable bitmap (WPA3 §13) that a
// network with transition_disable sends its clients, by its security
// (0297): WPA3-Personal on SAE, WPA3-Enterprise, Enhanced Open on OWE; ""
// where the network has no newer way to keep its clients to.
func TransitionDisable(security string) string {
	switch security {
	case "wpa3-sae", "wpa2-wpa3":
		return "0x01"
	case "wpa3-enterprise", "wpa2-wpa3-enterprise":
		return "0x04"
	case "owe":
		return "0x08"
	}
	return ""
}

// PMFChoice says whether a network's protected management frames are its
// own to set (0227): so on one that takes WPA2. WPA3 alone and OWE always
// require them, and an open network has none.
func PMFChoice(security string) bool {
	switch security {
	case "wpa2-psk", "wpa2-wpa3", "wpa2-enterprise", "wpa2-wpa3-enterprise":
		return true
	}
	return false
}

// Pair2G is the channel a 2.4 GHz channel is joined with at 40 MHz (0117):
// the one four above for 1 to 6, the one four below from 7 up, as OpenWrt
// joins a channel that is set, and as radio resource management moves to
// one.
func Pair2G(channel int) int {
	if channel < 7 {
		return channel + 4
	}
	return channel - 4
}

// usable2G is Usable on 2.4 GHz, whose channels overlap and come in no
// blocks: the set itself, or with spread only those far enough apart not to
// overlap, taken from the lowest. At 20 MHz that is five apart, as 1, 6 and
// 11 are. At 40 MHz a channel takes its pair too, and two such are apart
// when their middles are eight channels apart: 1 with 5, and 13 with 9.
func usable2G(set []int, width int, spread bool) []int {
	out := slices.Clone(set)
	slices.Sort(out)
	out = slices.Compact(out)
	if !spread {
		return out
	}
	// Twice the middle, to stay in whole numbers.
	middle, gap := func(c int) int { return 2 * c }, 10
	if width > 20 {
		middle, gap = func(c int) int { return c + Pair2G(c) }, 16
	}
	var kept []int
	for _, c := range out {
		if len(kept) == 0 || middle(c)-middle(kept[len(kept)-1]) >= gap {
			kept = append(kept, c)
		}
	}
	return kept
}

// Primary is the 20 MHz channel a radio on channel, at width, beacons on
// (0243): on 5 GHz at 40 MHz or more, its block's lowest; on 6 GHz, its
// block's second, which at 80 and 160 MHz is a preferred scanning channel.
// Elsewhere, and at 20 MHz, the channel itself. The same channel set at
// the same width is then the same primary everywhere, and hostapd is not
// left to choose: it once took a Living Room AP set to 149 onto 153.
func Primary(band string, channel, width int) int {
	blocks, _ := groups(band, width)
	if width <= 20 || (band != "5g" && band != "6g") {
		return channel
	}
	for _, g := range blocks {
		if channel >= g[0] && channel <= g[1] {
			return blockPrimary(band, g)
		}
	}
	return channel
}

// Usable is what an automatic channel on a band may be, the list hostapd is
// given (0075, 0087): the primaries (0243) of the set's whole blocks at the
// width, outside radar where it is avoided; on 6 GHz, with psc, only
// preferred scanning channels; and with spread, one block's primary to a
// block, each block apart from the others, so radios on different channels
// never share one. On 2.4 GHz, which has no blocks, spread keeps the
// channels that do not overlap (0117). Sorted.
func Usable(band string, set []int, width int, avoidRadar, psc, spread bool) []int {
	if band == "2g" {
		return usable2G(set, width, spread)
	}
	whole := Whole(band, set, width, avoidRadar)
	psc = psc && band == "6g" // preferred scanning channels are 6 GHz's
	ok := func(c int) bool { return !psc || PSC(c) }
	blocks, _ := groups(band, width)
	if len(blocks) == 0 || width <= 20 {
		var out []int
		for _, c := range whole {
			if ok(c) {
				out = append(out, c)
			}
		}
		return out
	}
	in := map[int]bool{}
	for _, c := range whole {
		in[c] = true
	}
	blocks = slices.Clone(blocks)
	slices.SortFunc(blocks, func(a, b [2]int) int { return a[0] - b[0] })
	var out []int
	last := 0 // the highest channel of the blocks taken, where spread
	for _, b := range blocks {
		p := blockPrimary(band, b)
		if !in[b[0]] || !in[b[1]] || !ok(p) || (spread && b[0] <= last) {
			continue
		}
		out = append(out, p)
		last = b[1]
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// blockPrimary is a block's primary channel (0243): its lowest on 5 GHz,
// its second on 6 GHz.
func blockPrimary(band string, b [2]int) int {
	if band == "6g" {
		return b[0] + 4
	}
	return b[0]
}
