package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Nothing Aeolus runs needs the internet (0069): the UI and the agent name
// no internet address. The few that are named are no place they fetch from.
var named = map[string]string{
	"http://www.w3.org/2000/svg":                "the SVG namespace, a name and not a place",
	"https://downloads.openwrt.org/releases/":   "OpenWrt's feeds, named only to point them at the manager",
	"https://downloads.openwrt.org/snapshots/":  "OpenWrt's main snapshots, named only to point them at the manager (0230)",
	"https://aeolus.example.com:8443":           "a manager, in install.sh's example",
	"https://192.168.20.60:8443":                "a manager by its private address, as 0040 advises, in aeolus-setup's and install.sh's examples (0080)",
	"https://www.google.com/maps/search/":       "a link a person may open, to see a pin's place in Google Maps (0169): the page fetches nothing from it",
	"https://platform.claude.com/settings/keys": "a link a person may open, to make the advisor's Anthropic API key (0205): the page fetches nothing from it",
	"https://github.com/chjj/term.js":           "xterm.css's copyright notice (0198): its authors' page, in a comment",
	"http://bellard.org/jslinux/":               "xterm.css's copyright notice (0198): its authors' page, in a comment",
	// plotly.js (0294), as its authors ship it.
	"http://www.w3.org/1999/xhtml":                     "the XHTML namespace, in Plotly's d3: a name and not a place",
	"http://www.w3.org/1999/xlink":                     "the XLink namespace, in Plotly's d3: a name and not a place",
	"http://www.w3.org/XML/1998/namespace":             "the XML namespace, in Plotly's d3: a name and not a place",
	"http://www.w3.org/2000/xmlns/":                    "the XMLNS namespace, in Plotly's d3: a name and not a place",
	"https://cloud.plotly.com/newchart":                "Plotly's plotlyServerURL default, used only by its send-to-cloud buttons, which are off and which chart.js never turns on",
	"https://cdn.plot.ly/un/":                          "Plotly's topojsonURL default, for map traces, which the basic bundle has none of",
	"https://plotly.com/":                              "the link on Plotly's logo, which chart.js leaves out (displaylogo false)",
	"https://plotly.com/javascript/animations/":        "a page named in Plotly's console warnings: it fetches nothing from it",
	"https://github.com/d3/d3-format/tree/v1.4.5":      "a page named in Plotly's attribute descriptions: it fetches nothing from it",
	"https://github.com/d3/d3-time-format/tree/v2.2.3": "a page named in Plotly's attribute descriptions: it fetches nothing from it",
	"http://getify.mit-license.org":                    "a licence notice in Plotly's bundle, in a comment",
	"http://syntheti.cc":                               "a licence notice in Plotly's bundle, in a comment",
	"https://github.com/voidqk/polybooljs":             "a licence notice in Plotly's bundle, in a comment",
}

var address = regexp.MustCompile(`https?://[A-Za-z0-9.\-]+(:[0-9]+)?(/[A-Za-z0-9._~/\-]*)?`)

func TestNoInternetAddresses(t *testing.T) {
	roots := []string{"static", "../../agent/files", "../../agent/install.sh"}
	seen := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			seen++
			for _, a := range address.FindAllString(string(b), -1) {
				if _, ok := named[a]; !ok {
					t.Errorf("%s names %s: nothing Aeolus runs may fetch from the internet (0069)", p, a)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen < 10 {
		t.Fatalf("looked at only %d files", seen)
	}
}
