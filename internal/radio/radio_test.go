package radio

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestModeWidth(t *testing.T) {
	for m, want := range map[string]int{"HT20": 20, "HT40": 40, "VHT80": 80, "VHT160": 160, "HE80": 80, "EHT320": 320, "VHT80+80": 0, "NOHT": 0, "": 0} {
		got, ok := ModeWidth(m)
		if got != want || ok != (want > 0) {
			t.Errorf("ModeWidth(%q) = %d, %v", m, got, ok)
		}
	}
	if got := Can([]string{"HT20", "HT40", "VHT80", "VHT80+80"}); !reflect.DeepEqual(got, map[int]bool{20: true, 40: true, 80: true}) {
		t.Errorf("Can = %v", got)
	}
}

func TestCanOnHoldsAModelToItsGrant(t *testing.T) {
	five := []string{"HT20", "HT40", "VHT20", "VHT40", "VHT80", "VHT160", "HE20", "HE40", "HE80", "HE160"}
	six := []string{"HE20", "HE40", "HE80", "HE80+80", "HE160"}
	if got := CanOn("arista,c-360", "5g", five); got[160] || !got[80] || !got[20] {
		t.Fatalf("C-360 5 GHz = %v, want 20-80 only", got)
	}
	if got := CanOn("arista,c-360", "6g", six); !got[160] {
		t.Fatalf("C-360 6 GHz = %v, want 160 kept", got)
	}
	if got := CanOn("openwrt,one", "5g", five); !got[160] {
		t.Fatalf("OpenWrt One 5 GHz = %v, want 160 kept", got)
	}
	if got := CanOn("", "5g", five); !got[160] {
		t.Fatalf("no board = %v, want what the radio reports", got)
	}
	if w, ok := GrantedMost("arista,c-360", "5g"); !ok || w != 80 {
		t.Fatalf("GrantedMost = %d %v", w, ok)
	}
	// A W-118's grant leaves out 144: at 20 MHz on it, at 40 from 140, and
	// at 80 from anywhere in 132–144; nothing else.
	for _, c := range []struct{ ch, w, want int }{{144, 20, 144}, {140, 40, 144}, {132, 80, 144}, {136, 20, 0}, {136, 40, 0}, {149, 80, 0}} {
		if got := OffIn("arista,w-118", "5g", c.ch, c.w); got != c.want {
			t.Errorf("OffIn(w-118, %d, %d) = %d, want %d", c.ch, c.w, got, c.want)
		}
	}
	if got := OffIn("arista,c-360", "5g", 144, 20); got != 0 {
		t.Errorf("OffIn(c-360, 144) = %d, want 0", got)
	}
}

func TestFits(t *testing.T) {
	cases := []struct {
		band       string
		channel, w int
		ok         bool
		why        string
	}{
		{"5g", 149, 80, true, ""},
		{"5g", 149, 160, false, "that needs a channel from 36–64 or 100–128"},
		{"5g", 36, 160, true, ""},
		{"5g", 165, 40, true, ""}, // with 169, an upper block (0232)
		{"5g", 177, 80, true, ""},
		{"5g", 165, 160, false, "that needs a channel from 36–64 or 100–128"},
		{"5g", 165, 20, true, ""},
		{"5g", 0, 160, true, ""},
		{"2g", 13, 40, true, ""},
	}
	for _, c := range cases {
		ok, why := Fits(c.band, c.channel, c.w)
		if ok != c.ok || !strings.Contains(why, c.why) {
			t.Errorf("Fits(%s, %d, %d) = %v %q", c.band, c.channel, c.w, ok, why)
		}
	}
}

func TestRadar(t *testing.T) {
	for _, c := range []struct {
		band       string
		channel, w int
		want       bool
	}{
		{"5g", 36, 160, true}, // 36–64 includes 52–64
		{"5g", 36, 80, false},
		{"5g", 52, 20, true},
		{"5g", 48, 40, false},
		{"5g", 100, 80, true},
		{"5g", 149, 80, false},
		{"5g", 0, 160, true}, // automatic: every 160 MHz block has radar channels
		{"5g", 0, 80, false}, // 36–48 and 149–161 do not
		{"5g", 0, 20, false},
		{"2g", 6, 40, false},
	} {
		if got := Radar(c.band, c.channel, c.w); got != c.want {
			t.Errorf("Radar(%s, %d, %d) = %v, want %v", c.band, c.channel, c.w, got, c.want)
		}
	}
}

// A set keeps only whole blocks at the width on 5 GHz (0075): hostapd
// checks only a block's primary channel against its list.
func TestWhole(t *testing.T) {
	set := []int{36, 40, 44, 48, 52, 56, 149, 153, 165}
	for _, c := range []struct {
		band  string
		width int
		avoid bool
		want  []int
	}{
		{"5g", 20, false, []int{36, 40, 44, 48, 52, 56, 149, 153, 165}},
		{"5g", 20, true, []int{36, 40, 44, 48, 149, 153, 165}},
		{"5g", 40, false, []int{36, 40, 44, 48, 52, 56, 149, 153}},
		{"5g", 40, true, []int{36, 40, 44, 48, 149, 153}},
		{"5g", 80, false, []int{36, 40, 44, 48}},
		{"5g", 160, false, nil},
		{"2g", 20, true, []int{36, 40, 44, 48, 52, 56, 149, 153, 165}},
	} {
		if got := Whole(c.band, set, c.width, c.avoid); !slices.Equal(got, c.want) {
			t.Errorf("Whole(%s, %d MHz, avoid %v) = %v, want %v", c.band, c.width, c.avoid, got, c.want)
		}
	}
	if got := Whole("2g", []int{11, 1, 6, 6}, 20, false); !slices.Equal(got, []int{1, 6, 11}) {
		t.Errorf("2.4 GHz: %v", got)
	}
}

// 6 GHz (0087): blocks of 40, 80, 160 and 320 MHz across 1–233, two
// overlapping families at 320; the preferred scanning channels.
func TestSixGHz(t *testing.T) {
	if len(Channels6) != 59 || Channels6[0] != 1 || Channels6[58] != 233 {
		t.Fatalf("channels %v", Channels6)
	}
	for ch, want := range map[int]bool{5: true, 21: true, 37: true, 229: true, 1: false, 33: false, 233: false, 213: true} {
		if PSC(ch) != want {
			t.Errorf("PSC(%d) = %v", ch, !want)
		}
	}
	if got := PSCOnly(Channels6); len(got) != 15 {
		t.Errorf("15 preferred scanning channels, got %v", got)
	}
	for _, c := range []struct {
		ch, width int
		ok        bool
	}{{1, 160, true}, {29, 160, true}, {225, 160, false}, {233, 40, false}, {229, 40, true}, {221, 80, true}, {225, 80, false}, {93, 320, true}, {189, 320, true}, {225, 320, false}} {
		if ok, why := Fits("6g", c.ch, c.width); ok != c.ok {
			t.Errorf("Fits(6g, %d, %d) = %v %q", c.ch, c.width, ok, why)
		}
	}
	// 1–29 is a whole 160 MHz block, 33–57 is not; at 320 the two
	// families overlap, so 1–93 holds two whole blocks.
	set := []int{1, 5, 9, 13, 17, 21, 25, 29, 33, 37, 41, 45, 49, 53, 57}
	if got := Whole("6g", set, 160, false); len(got) != 8 || got[7] != 29 {
		t.Errorf("Whole 160 = %v", got)
	}
	var low []int
	for c := 1; c <= 93; c += 4 {
		low = append(low, c)
	}
	if got := Whole("6g", low, 320, false); len(got) != 24 || got[0] != 1 || got[23] != 93 {
		t.Errorf("Whole 320 = %v", got)
	}
}

// Usable (0087): preferred scanning channels only, and one channel to a
// block, so 160 MHz radios on different channels never share a block.
// 2.4 GHz has no blocks (0117): the set as it is, or with spread the
// channels of it that do not overlap at the width.
func TestUsable2G(t *testing.T) {
	all11 := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	all13 := append(slices.Clone(all11), 12, 13)
	for _, c := range []struct {
		name   string
		set    []int
		width  int
		spread bool
		want   []int
	}{
		{"the set as it is", []int{11, 1, 6}, 20, false, []int{1, 6, 11}},
		{"the set as it is, at 40", []int{1, 6, 11}, 40, false, []int{1, 6, 11}},
		{"apart, at 20, of 1 to 11", all11, 20, true, []int{1, 6, 11}},
		{"apart, at 20, of 1 to 13", all13, 20, true, []int{1, 6, 11}},
		{"apart, at 20, of 3 4 8 9", []int{3, 4, 8, 9}, 20, true, []int{3, 8}},
		{"apart, at 40, of 1 6 11", []int{1, 6, 11}, 40, true, []int{1}},
		{"apart, at 40, of 1 to 11", all11, 40, true, []int{1}},
		{"apart, at 40, of 1 to 13", all13, 40, true, []int{1, 13}},
		{"apart, at 40, of 6 11 13", []int{6, 11, 13}, 40, true, []int{6}},
	} {
		if got := Usable("2g", c.set, c.width, false, false, c.spread); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	for c, want := range map[int]int{1: 5, 6: 10, 7: 3, 11: 7, 13: 9} {
		if got := Pair2G(c); got != want {
			t.Errorf("channel %d is joined with %d, want %d", c, got, want)
		}
	}
}

func TestUsable(t *testing.T) {
	for _, c := range []struct {
		name        string
		width       int
		psc, spread bool
		want        []int
	}{
		// A block's primary is its second channel (0243): at 160 MHz, the
		// lower 80's preferred scanning channel.
		{"psc at 160", 160, true, false, []int{5, 37, 69, 101, 133, 165, 197}},
		{"psc, one to a block, at 160", 160, true, true, []int{5, 37, 69, 101, 133, 165, 197}},
		{"one to a block at 160", 160, false, true, []int{5, 37, 69, 101, 133, 165, 197}},
		{"psc at 80, one to a block anyway", 80, true, true, []int{5, 21, 37, 53, 69, 85, 101, 117, 133, 149, 165, 181, 197, 213}},
		{"psc, one to a block, at 320: one family", 320, true, true, []int{5, 69, 133}},
	} {
		if got := Usable("6g", Channels6, c.width, false, c.psc, c.spread); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// On 5 GHz psc means nothing, and a block's primary is its lowest
	// (0243).
	if got := Usable("5g", []int{36, 40, 44, 48}, 80, false, true, false); !slices.Equal(got, []int{36}) {
		t.Errorf("5 GHz at 80: %v", got)
	}
	if got := Usable("5g", []int{36, 40, 44, 48, 149, 153, 157, 161}, 40, false, false, false); !slices.Equal(got, []int{36, 44, 149, 157}) {
		t.Errorf("5 GHz at 40: %v", got)
	}
	if got := Usable("5g", []int{36, 40, 44, 48}, 20, false, false, false); !slices.Equal(got, []int{36, 40, 44, 48}) {
		t.Errorf("5 GHz at 20: %v", got)
	}
}

// A radio's primary channel is its block's lowest on 5 GHz and its second
// on 6 GHz (0243), whatever channel of the block is set.
func TestPrimary(t *testing.T) {
	for _, c := range []struct {
		band                  string
		channel, width, prime int
	}{
		{"5g", 153, 40, 149}, {"5g", 149, 40, 149}, {"5g", 161, 80, 149}, {"5g", 48, 80, 36}, {"5g", 64, 160, 36},
		{"5g", 153, 20, 153}, {"5g", 165, 40, 165},
		{"6g", 1, 80, 5}, {"6g", 13, 80, 5}, {"6g", 21, 160, 5}, {"6g", 37, 160, 37}, {"6g", 9, 40, 13}, {"6g", 9, 20, 9},
		{"2g", 11, 40, 11},
	} {
		if got := Primary(c.band, c.channel, c.width); got != c.prime {
			t.Errorf("%s channel %d at %d MHz: primary %d, want %d", c.band, c.channel, c.width, got, c.prime)
		}
	}
}

// A band's generations are one unbroken run of its own (0089); what a radio
// serves runs up to its newest family.
func TestGenerations(t *testing.T) {
	for _, c := range []struct {
		band           string
		modes          []string
		oldest, newest string
		err            string
	}{
		{"2g", []string{"g", "n", "ax"}, "g", "ax", ""},
		{"2g", []string{"ax", "b", "n", "g"}, "b", "ax", ""},
		{"5g", []string{"ac"}, "ac", "ac", ""},
		{"5g", []string{"a", "ac"}, "", "", "802.11a and 802.11ac are allowed, but not 802.11n between them"},
		{"5g", []string{"g"}, "", "", "5 GHz has no 802.11g"},
		{"6g", []string{"n", "ax"}, "", "", "6 GHz has no 802.11n"},
		{"6g", nil, "", "", "no 802.11 generation is allowed"},
	} {
		oldest, newest, err := Span(c.band, c.modes)
		if oldest != c.oldest || newest != c.newest || (err == nil) != (c.err == "") || err != nil && err.Error() != c.err {
			t.Errorf("Span(%s, %v) = %q, %q, %v; want %q, %q, %q", c.band, c.modes, oldest, newest, err, c.oldest, c.newest, c.err)
		}
	}
	if got := Serves("5g", []string{"HT20", "HT40", "VHT20", "VHT80"}); !slices.Equal(got, []string{"a", "n", "ac"}) {
		t.Errorf("an 802.11ac radio serves %v", got)
	}
	if got := Serves("2g", []string{"HE20", "HE40"}); !slices.Equal(got, []string{"b", "g", "n", "ax"}) {
		t.Errorf("an 802.11ax radio on 2.4 GHz serves %v", got)
	}
	if got := Serves("6g", nil); got != nil {
		t.Errorf("a radio that reported nothing serves %v", got)
	}
	for band, want := range map[string]string{"2g": "ax", "5g": "ax", "6g": ""} {
		if got := Required(band, "ax"); got != want {
			t.Errorf("Required(%s, ax) = %q, want %q", band, got, want)
		}
	}
	if Required("5g", "a") != "" || Required("2g", "n") != "n" || Required("5g", "be") != "" {
		t.Error("require_mode is OpenWrt's n, ac or ax, for the oldest after the band's first")
	}
}

// TestEdges: a block's edges in MHz (0327). The Living Room C-360's 5 GHz on
// 157/40 is 5775-5815; its 6 GHz on 5/160 is 5945-6105, and on 37/160
// 6105-6265.
func TestEdges(t *testing.T) {
	for _, c := range []struct {
		band           string
		channel, width int
		lo, hi         int
	}{
		{"5g", 157, 40, 5775, 5815}, {"5g", 149, 80, 5735, 5815}, {"5g", 36, 20, 5170, 5190},
		{"6g", 5, 160, 5945, 6105}, {"6g", 37, 160, 6105, 6265}, {"6g", 5, 20, 5965, 5985}, {"6g", 2, 20, 5925, 5945},
	} {
		if lo, hi := Edges(c.band, c.channel, c.width); lo != c.lo || hi != c.hi {
			t.Errorf("Edges(%s, %d, %d) = %d-%d, want %d-%d", c.band, c.channel, c.width, lo, hi, c.lo, c.hi)
		}
	}
}

// How a 5 GHz radio uses the DFS channels (0328): avoid avoids them; allow,
// as unset and look-first, looks where an automatic channel has RRM moving
// radios and a whole block outside DFS at its width among its channels,
// and picks otherwise.
func TestDFSWay(t *testing.T) {
	for _, c := range []struct {
		setting     string
		auto, moves bool
		channels    []int
		width       int
		want        string
	}{
		{"avoid", true, true, nil, 80, DFSAvoid},
		{"avoid", false, false, nil, 160, DFSAvoid},
		{"allow", true, true, nil, 80, DFSLook},
		{"", true, true, nil, 40, DFSLook},
		{"look-first", true, true, nil, 20, DFSLook},
		{"allow", true, false, nil, 80, DFSPick}, // RRM moves no radio: nothing would look
		{"allow", false, true, nil, 80, DFSPick}, // a channel set by hand is used as set
		{"allow", true, true, nil, 160, DFSPick}, // every 160 MHz block holds DFS channels
		{"allow", true, true, []int{36, 100}, 160, DFSPick},
		{"allow", true, true, []int{52, 100}, 40, DFSPick}, // only DFS channels in the set
		{"allow", true, true, []int{52, 149}, 80, DFSLook},
		{"allow", true, true, []int{149}, 160, DFSPick}, // in no 160 MHz block
	} {
		if got := DFSWay(c.setting, c.auto, c.moves, c.channels, c.width); got != c.want {
			t.Errorf("DFSWay(%q, auto %v, moves %v, %v, %d) = %s, want %s", c.setting, c.auto, c.moves, c.channels, c.width, got, c.want)
		}
	}
}

// A radio set to 160 MHz with DFS allowed starts at 80 MHz outside DFS
// where RRM moves radios and its set has such a block (0329).
func TestStartWidth(t *testing.T) {
	for _, c := range []struct {
		setting     string
		width       int
		auto, moves bool
		set         []int
		want        int
	}{
		{"allow", 160, true, true, nil, 80},
		{"", 160, true, true, nil, 80},
		{"look-first", 160, true, true, nil, 80},
		{"allow", 160, true, true, []int{36, 40, 44, 48, 52, 56, 60, 64}, 80},
		{"allow", 160, true, true, []int{52, 56, 60, 64, 100, 104, 108, 112}, 160}, // no 80 MHz block outside DFS
		{"allow", 160, true, false, nil, 160},                                      // RRM moves no radio
		{"allow", 160, false, true, nil, 160},                                      // a channel set by hand
		{"avoid", 160, true, true, nil, 160},
		{"allow", 80, true, true, nil, 80},
		{"allow", 40, true, true, nil, 40},
	} {
		if got := StartWidth(c.setting, c.width, c.auto, c.moves, c.set); got != c.want {
			t.Errorf("StartWidth(%q, %d, auto %v, moves %v, %v) = %d, want %d", c.setting, c.width, c.auto, c.moves, c.set, got, c.want)
		}
	}
}
