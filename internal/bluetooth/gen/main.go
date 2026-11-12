// Command gen makes sig.txt.gz, the Bluetooth SIG's assigned numbers the
// manager names what an AP's Bluetooth radio hears by (0157): one line a
// number, its kind, a tab, its hex digits, a tab, and its name, sorted. The
// kinds are c, a company's identifier, which starts its manufacturer data;
// u, a 16-bit UUID, a service's or one assigned to a member; and a, a
// category of appearance.
//
//	go run ./internal/bluetooth/gen [directory]
//
// The directory holds company_identifiers.yaml, member_uuids.yaml,
// service_uuids.yaml and appearance_values.yaml, from the SIG's public
// repository; without one they are fetched from it.
package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const source = "https://bitbucket.org/bluetooth-SIG/public/raw/main/assigned_numbers/"

// The files, where each is in the SIG's repository, what each line that
// begins an entry says, and the kind it is given here.
var files = []struct {
	name, path string
	key        *regexp.Regexp
	kind       string
}{
	{"company_identifiers.yaml", "company_identifiers/", regexp.MustCompile(`^\s*- value: 0x([0-9A-Fa-f]{4})\s*$`), "c"},
	{"member_uuids.yaml", "uuids/", regexp.MustCompile(`^\s*- uuid: 0x([0-9A-Fa-f]{4})\s*$`), "u"},
	{"service_uuids.yaml", "uuids/", regexp.MustCompile(`^\s*- uuid: 0x([0-9A-Fa-f]{4})\s*$`), "u"},
	// A category, not the subcategories under it, which are indented more.
	{"appearance_values.yaml", "core/", regexp.MustCompile(`^ - category: 0x([0-9A-Fa-f]{3})\s*$`), "a"},
}

var nameRE = regexp.MustCompile(`^\s*name: (.*)$`)

// unquote reads a YAML scalar as these files write them: plain, in single
// quotes, a quote in it written twice, or in double quotes.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	if len(s) >= 2 && s[0] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

func open(dir, name, path string) (io.ReadCloser, error) {
	if dir != "" {
		return os.Open(filepath.Join(dir, name))
	}
	resp, err := http.Get(source + path + name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", name, resp.Status)
	}
	return resp.Body, nil
}

func main() {
	dir := ""
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	seen := map[string]bool{}
	var lines []string
	for _, f := range files {
		in, err := open(dir, f.name, f.path)
		if err != nil {
			log.Fatal(err)
		}
		s := bufio.NewScanner(in)
		key, n := "", 0
		for s.Scan() {
			if m := f.key.FindStringSubmatch(s.Text()); m != nil {
				key = f.kind + "\t" + strings.ToUpper(m[1])
				continue
			}
			if m := nameRE.FindStringSubmatch(s.Text()); m != nil && key != "" {
				// The first name a number is given is kept: the member's,
				// where a service has the same number.
				name := strings.Join(strings.Fields(unquote(m[1])), " ")
				if name != "" && !seen[key] && !strings.ContainsAny(name, "\t\n") {
					seen[key] = true
					lines = append(lines, key+"\t"+name)
					n++
				}
				key = ""
			}
		}
		in.Close()
		if err := s.Err(); err != nil {
			log.Fatal(err)
		}
		if n == 0 {
			log.Fatalf("%s: nothing read", f.name)
		}
		log.Printf("%s: %d", f.name, n)
	}
	sort.Strings(lines)
	out, err := os.Create(filepath.Join("internal", "bluetooth", "sig.txt.gz"))
	if err != nil {
		log.Fatal(err)
	}
	z, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	for _, l := range lines {
		fmt.Fprintln(z, l)
	}
	if err := z.Close(); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d numbers", len(lines))
}
