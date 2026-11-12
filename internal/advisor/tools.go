package advisor

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// leftOut is the read-only tools a review does not have (0205): the
// preview of a change, which is no read of the network; an AP's whole
// history, whose answer is too large to be of use in a review; and its own
// findings, which its first message tells it.
var leftOut = []string{"preview_change", "get_ap_history", "get_advice"}

// resultMax is the most of a tool's answer a review reads, in characters:
// past it, the answer is cut, and the review told to ask for less.
const resultMax = 40000

// reads are the manager's reads a review has, through the MCP adapter, as
// the account the review reads as.
type reads struct {
	session *mcp.ClientSession
	tools   []*mcp.Tool
}

// openReads connects to the adapter's server in the process, and keeps the
// tools marked read-only, but those left out.
func openReads(ctx context.Context, srv *mcp.Server) (*reads, func(), error) {
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return nil, nil, err
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "aeolus-advisor", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		return nil, nil, err
	}
	closeAll := func() {
		cs.Close()
		ss.Close()
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		closeAll()
		return nil, nil, err
	}
	r := &reads{session: cs}
	for _, t := range list.Tools {
		if t.Annotations != nil && t.Annotations.ReadOnlyHint && !slices.Contains(leftOut, t.Name) {
			r.tools = append(r.tools, t)
		}
	}
	sort.Slice(r.tools, func(i, j int) bool { return r.tools[i].Name < r.tools[j].Name })
	return r, closeAll, nil
}

// has says whether name is one of the reads.
func (r *reads) has(name string) bool {
	return slices.ContainsFunc(r.tools, func(t *mcp.Tool) bool { return t.Name == name })
}

// params are the reads as tools for the model, in name order, so the list
// is the same at every request and is cached with the instructions.
func (r *reads) params() []anthropic.BetaToolUnionParam {
	out := make([]anthropic.BetaToolUnionParam, 0, len(r.tools))
	for _, t := range r.tools {
		schema, _ := t.InputSchema.(map[string]any)
		in := anthropic.BetaToolInputSchemaParam{ExtraFields: map[string]any{}}
		for k, v := range schema {
			switch k {
			case "type":
			case "properties":
				in.Properties = v
			case "required":
				for _, s := range asStrings(v) {
					in.Required = append(in.Required, s)
				}
			default:
				in.ExtraFields[k] = v
			}
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: in,
		}})
	}
	return out
}

func asStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// call makes one read, with the model's input, and gives its answer as
// text, cut at resultMax; and whether it is an error.
func (r *reads) call(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	if !r.has(name) {
		return fmt.Sprintf("there is no tool %q", name), true
	}
	var args map[string]any
	if len(input) > 0 && string(input) != "null" {
		if err := json.Unmarshal(input, &args); err != nil {
			return "the input is not a JSON object: " + err.Error(), true
		}
	}
	res, err := r.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error(), true
	}
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	text := b.String()
	if text == "" && res.StructuredContent != nil {
		raw, _ := json.Marshal(res.StructuredContent)
		text = string(raw)
	}
	return cut(text), res.IsError
}

// cut keeps the first resultMax characters of an answer, and says so.
func cut(text string) string {
	if len(text) <= resultMax {
		return text
	}
	// Not in the middle of a character.
	end := resultMax
	for end > 0 && !utf8Start(text[end]) {
		end--
	}
	return text[:end] + fmt.Sprintf("\n… cut at %d of its %d characters: ask for less, with under, hours, a narrower search or one client", end, len(text))
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// recordFinding is the review's one tool that writes: a finding, in the
// manager's conditions store (0205). Its schema is strict, so what comes
// is what it says.
func recordFinding() anthropic.BetaToolUnionParam {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	list := func(desc string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
	}
	return anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
		Name: "record_finding",
		Description: anthropic.String("Record one problem you found, for the people who run this network to read and act on. " +
			"Call it once for each problem, with the evidence you found it by. A problem found before, by its key, is updated, not repeated."),
		Strict: anthropic.Bool(true),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"key": str("A short name for the problem that stays the same when it is found again, in lowercase words and hyphens: " +
					"what and where, such as iot-devices-rejoin-c360-2g or wan-drops-large-packets. Use a key from the findings already recorded where it is the same problem."),
				"kind":     map[string]any{"type": "string", "enum": []string{"client", "compatibility", "ap", "network", "config"}, "description": "client: one client or a few; compatibility: a maker's clients with a kind of AP or band; ap: an AP or the manager misbehaving; network: a network's service, DHCP, DNS or the way out; config: a setting that does harm."},
				"severity": map[string]any{"type": "string", "enum": []string{"info", "warning", "critical"}, "description": "critical: clients are without service now; warning: clients are hurt, or will be; info: worth knowing, nobody is hurt yet."},
				"title":    str("One line saying the problem, such as: Two IoT devices rejoin every 45 s on the C-360s' 2.4 GHz"),
				"saw":      str("What you saw, with the numbers and where you saw them (which tool, which AP, which client, over what time), so a person can check it."),
				"todo":     str("What to do about it, as plainly and specifically as you can: which setting, on which folder or AP, to what; or what to look at next, where you cannot tell."),
				"clients":  list("The clients it concerns, by MAC."),
				"aps":      list("The APs it concerns, by ID (ap-...)."),
				"networks": list("The networks it concerns, by ID."),
			},
			Required:    []string{"key", "kind", "severity", "title", "saw", "todo", "clients", "aps", "networks"},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}}
}

// resolveFinding closes a finding whose problem a review found gone
// (0215): the finding's key, and what it saw instead.
func resolveFinding() anthropic.BetaToolUnionParam {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	return anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
		Name: "resolve_finding",
		Description: anthropic.String("Resolve an open finding recorded before whose problem is gone now. " +
			"Only where you looked and saw it gone; where you could not tell, leave it open. If the problem comes back, recording it again opens it again."),
		Strict: anthropic.Bool(true),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"key": str("The key of the open finding."),
				"why": str("What you saw that says the problem is gone, with the numbers and where (which tool, over what time), so a person can check it."),
			},
			Required:    []string{"key", "why"},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}}
}

// resolution is resolve_finding's input.
type resolution struct {
	Key string `json:"key"`
	Why string `json:"why"`
}

// finding is record_finding's input.
type finding struct {
	Key      string   `json:"key"`
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Saw      string   `json:"saw"`
	Todo     string   `json:"todo"`
	Clients  []string `json:"clients"`
	APs      []string `json:"aps"`
	Networks []string `json:"networks"`
}
