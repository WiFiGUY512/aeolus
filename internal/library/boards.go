package library

// Boards OpenWrt renamed (0174): each board's earlier name, by the name it
// has now. An AP that reports either is the same kind of AP, and everything
// kept by board (a template's boards, templates.<board>, boards.<board>.)
// goes by the name it has now. The Arista boards were named arista,c360 and
// so on before OpenWrt's review gave them their models' names.
var renamed = map[string]string{
	"arista,c260": "arista,c-260",
	"arista,c360": "arista,c-360",
	"arista,w318": "arista,w-318",
}

// Board is b by the name OpenWrt gives it now: a renamed board by its
// newest name, through every rename since, any other as it is.
func Board(b string) string {
	for range len(renamed) { // a chain of renames is no longer than the list
		n, ok := renamed[b]
		if !ok {
			break
		}
		b = n
	}
	return b
}

// Boards is boards by the names OpenWrt gives them now, each once, in the
// order given.
func Boards(boards []string) []string {
	if boards == nil {
		return nil
	}
	out := make([]string, 0, len(boards))
	seen := make(map[string]bool, len(boards))
	for _, b := range boards {
		b = Board(b)
		if !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	return out
}
