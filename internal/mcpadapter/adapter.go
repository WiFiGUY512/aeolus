// Package mcpadapter serves the manager's API as MCP tools at /mcp (0028,
// 0031).
//
// It is a pass-through: every MCP request must carry the caller's own Aeolus
// token, and the adapter makes its API calls with that same token, through the
// API's HTTP handler in the same process. It holds no credentials and has no
// path to the state except the API (0003), so each MCP client acts, and is
// logged, under its own identity.
package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const instructions = `Aeolus is the manager for an OpenWrt-based Wi-Fi system. It holds what each AP should run; APs do the rest themselves.

Two trees sit under the Org: "locations" (where APs are: radios, system, ports, and which service folders apply) and "services" (what APs offer: networks and their transports). Settings inherit down each tree field by field; a value shows where it comes from (self, inherited, locked, baseline). Locks stop overrides below; Break Hierarchy starts a new branch.

Every change is one logged entry with your name and the time; a reason is optional, a note for where the change does not speak for itself. Run preview_change before make_change for anything that re-versions APs, locks, moves or breaks hierarchy. Secrets such as passphrases are sealed; you can set them but never read them back.`

// New returns the /mcp handler. api is the API's handler; the adapter is its
// client.
func New(api http.Handler, version string) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server(client{api: api, auth: r.Header.Get("Authorization")}, version)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Refuse a request without a working token before any MCP handling.
		c := client{api: api, auth: r.Header.Get("Authorization")}
		if _, err := c.call(r.Context(), "GET", "/v1/whoami", nil); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":"missing or invalid token"}`)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// Server is the adapter's MCP server over api, with no Authorization of its
// own: for the manager's use in its own process, where api is the API as
// one of its accounts (api.As), as the advisor reads it (0205).
func Server(api http.Handler, version string) *mcp.Server {
	return server(client{api: api}, version)
}

// client calls the API in-process with one caller's Authorization header.
type client struct {
	api  http.Handler
	auth string
}

// apiError is an error the API returned.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status) }

func (c client) call(ctx context.Context, method, path string, body any) (any, error) {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.api.ServeHTTP(rec, req)
	var out any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("API returned %d with a body that is not JSON", rec.Code)
	}
	if rec.Code >= 400 {
		msg := rec.Body.String()
		if m, ok := out.(map[string]any); ok {
			if e, ok := m["error"].(string); ok {
				msg = e
			}
		}
		return nil, &apiError{Status: rec.Code, Message: msg}
	}
	return out, nil
}

// Op is a change, as the model writes it.
type Op struct {
	Kind     string           `json:"kind" jsonschema:"one of: add-folder, remove-folder, add-ap, remove-ap, readmit-ap (an AP that enrolled again, as after a reflash, let back in as it was: 0223), replace-ap (an AP waiting in Landing Zone, named by with, put in node's place, which keeps its name, settings and records: 0223), rename, move, set, unset, lock, unlock, break-hierarchy, assign-services, add-account, set-account, disable-account, enable-account, remove-account, set-owner, grant, revoke, revoke-token, add-key, set-key, remove-key, add-template, edit-template, set-template, unset-template, remove-template, batch. The library's concentrator kinds are retired (0085)."`
	Tree     string           `json:"tree,omitempty" jsonschema:"locations or services"`
	Node     string           `json:"node,omitempty" jsonschema:"the folder or AP the change targets; for add-folder and add-ap, the new node's ID"`
	Parent   string           `json:"parent,omitempty" jsonschema:"parent folder, for add-folder, add-ap and move; for add-template, the Locations folder (or the Org) it is made at, offered there and below"`
	Name     string           `json:"name,omitempty" jsonschema:"display name, for add-folder, add-ap, add-account, set-account, add-template and edit-template"`
	Path     string           `json:"path,omitempty" jsonschema:"field path, e.g. radio.5g.width or network.sweet.transport.primary.vlan; a kind of AP's own port setting is boards.<board>.ports.<name>.<field>, such as boards.netgear,r7800.ports.lan2.mode (0092)"`
	Value    any              `json:"value,omitempty" jsonschema:"the field's new value, for set; for set-concentrator the definition {name, address, port, mtu, scope}; for add-key and set-key the key {name, passphrase, vlan, macs, expires}, its passphrase in plain text, which the manager seals (set-key may leave it out to keep it)"`
	Values   map[string]any   `json:"values,omitempty" jsonschema:"several fields of one node to set together, for set, in place of path and value: {path: value}; all or none are set"`
	Paths    []string         `json:"paths,omitempty" jsonschema:"several fields of one node to unset together, for unset, in place of path; all or none are unset"`
	Ops      []map[string]any `json:"ops,omitempty" jsonschema:"for batch: up to 256 sets and unsets, each written as a set or unset alone is, with its own tree and node, made in order as one change: all or none (0203); add-folder and assign-services too, so a new services folder, a location taking it and a network in it are one change (0320)"`
	Services []string         `json:"services,omitempty" jsonschema:"service folder IDs, for assign-services on a Locations node"`
	Account  string           `json:"account,omitempty" jsonschema:"account ID, for add-account, set-account, disable-account, enable-account, remove-account, set-owner, grant and revoke"`
	Role     string           `json:"role,omitempty" jsonschema:"admin, operator, manager, installer or viewer, for grant and revoke (0177)"`
	TokenID  string           `json:"token_id,omitempty" jsonschema:"token ID, for revoke-token"`
	Owner    *bool            `json:"owner,omitempty" jsonschema:"for set-owner: true makes the account an owner, who runs the Org's people; false unmakes it (0176)"`

	Concentrator string `json:"concentrator,omitempty" jsonschema:"retired: the library's concentrators take no new changes (0085)"`
	VNI          int    `json:"vni,omitempty" jsonschema:"retired, with the library's concentrators (0085)"`

	Template string   `json:"template,omitempty" jsonschema:"for the template kinds: the AP template's ID (0085); for set-template and unset-template its fields go in path and value, values or paths, as for set and unset, and add-template may carry values to make it with its settings (0090); and are Locations fields of radio, ports, system (not system.agent), rrm and apc. A folder picks a template for a board with set, path templates.<board>, value the template's ID"`
	Boards   []string `json:"boards,omitempty" jsonschema:"for add-template and edit-template: the boards it is for, as OpenWrt names them (arista,c-360)"`
	Default  bool     `json:"default,omitempty" jsonschema:"for add-template: also pick it where it is made, for each of its boards nothing is picked for there"`

	Network string `json:"network,omitempty" jsonschema:"for the key kinds: the network, as the Services folder in node offers it"`
	With    string `json:"with,omitempty" jsonschema:"for replace-ap: the AP waiting in Landing Zone to put in node's place (0223)"`
	Key     string `json:"key,omitempty" jsonschema:"for the key kinds: the key's ID; add-key makes one if it is left out"`
	Until   string `json:"until,omitempty" jsonschema:"optional: a change that ends itself (0345), for a set, unset, add-folder, assign-services or a batch of them: when, RFC 3339 (2026-11-10T22:35:00Z), a time to come at most 31 days off. Then the manager puts back what it did, field by field, wherever nothing has changed it since, and takes away the folders it made: a test window's maintenance, its channel pins and a services folder of its own, gone on time; list_ends lists them, end_change ends one sooner or moves its end"`
}

type endIn struct {
	Change int64          `json:"change" jsonschema:"the change that ends itself, by its sequence number"`
	Until  string         `json:"until,omitempty" jsonschema:"optional: a new end, RFC 3339, a time to come at most 31 days off; without it, the change ends now"`
	Node   string         `json:"node,omitempty" jsonschema:"optional, with until: a Locations node whose fields in values are set with the move, as part of the change, so they go at its end too (a test window's maintenance.until)"`
	Values map[string]any `json:"values,omitempty" jsonschema:"optional, with until and node: {path: value}, set as a set of them is, checked and sealed"`
	Reason string         `json:"reason,omitempty" jsonschema:"optional: a note for the change log"`
}

type journeyIn struct {
	MAC   string `json:"mac" jsonschema:"the client's MAC, such as aa:bb:cc:00:11:22"`
	Hours int    `json:"hours,omitempty" jsonschema:"optional: how many hours back, 24 unless set, at most 720"`
}

type traceIn struct {
	MAC     string `json:"mac" jsonschema:"the client's MAC, such as aa:bb:cc:00:11:22: one that is on, or not, or never seen"`
	Do      string `json:"do,omitempty" jsonschema:"optional: start, to trace it or go on tracing it; stop, to end its trace; unset, to read it"`
	Minutes int    `json:"minutes,omitempty" jsonschema:"optional, with start: for how many minutes from now, 15 unless set, at most 60"`
	After   int64  `json:"after,omitempty" jsonschema:"optional, reading: the next a read before gave, to have only what came since"`
}

type clientsIn struct {
	Under  string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only clients last seen on the APs below it"`
	Find   string `json:"find,omitempty" jsonschema:"optional: part of a MAC, a host name, a user or an address"`
	Limit  int    `json:"limit,omitempty" jsonschema:"optional: how many, 100 unless set, at most 500"`
	Offset int    `json:"offset,omitempty" jsonschema:"optional: how many to pass over, to page through"`
}

type connectionsIn struct {
	MAC    string `json:"mac" jsonschema:"the client's MAC, such as aa:bb:cc:00:11:22"`
	Limit  int    `json:"limit,omitempty" jsonschema:"optional: how many attempts, 50 unless set, at most 200"`
	Before string `json:"before,omitempty" jsonschema:"optional: an attempt's started time, to page back from"`
}

type usageIn struct {
	Under string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only the APs below it"`
	Hours int    `json:"hours,omitempty" jsonschema:"optional: how many hours back, 24 unless set, at most 720; for get_usage and get_network at most 8760, a year"`
}

type leavingIn struct {
	Under string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only the APs below it"`
	MAC   string `json:"mac,omitempty" jsonschema:"optional: one client, by its MAC"`
	Band  string `json:"band,omitempty" jsonschema:"optional: 2g, 5g or 6g; one band"`
	Hours int    `json:"hours,omitempty" jsonschema:"optional: how many hours back, 24 unless set, at most 2160"`
}

type minutesIn struct {
	Under string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only the APs below it"`
	Band  string `json:"band,omitempty" jsonschema:"optional: 2g, 5g or 6g; only that band's radios"`
	MAC   string `json:"mac,omitempty" jsonschema:"optional: a client's MAC; its minutes, in place of the radios'"`
	Hours int    `json:"hours,omitempty" jsonschema:"optional: how many hours back, 1 unless set; at most 24 for radios, 168 for a client"`
}

type recordingIn struct {
	ID int64 `json:"id" jsonschema:"a recording's id, as list_recordings gives it"`
}

type roundIn struct {
	Under string `json:"under" jsonschema:"a Locations folder or AP: every AP at or below it tests each thing it can once, now"`
	Over  string `json:"over,omitempty" jsonschema:"optional: which tests (0131): air, through the air with a scan radio; wire, over the wire from the uplink; or both, which it is unless said"`
	// What it is narrowed to (0188).
	Networks []string `json:"networks,omitempty" jsonschema:"optional: only these networks, by ID (0188)"`
	Bands    []string `json:"bands,omitempty" jsonschema:"optional: only these bands through the air: 2g, 5g, 6g (0188)"`
	On       []string `json:"on,omitempty" jsonschema:"optional: only what joins on these APs, by ID: the APs to test (0188)"`
	Only     []struct {
		Network string `json:"network"`
		On      string `json:"on"`
		Band    string `json:"band"`
	} `json:"only,omitempty" jsonschema:"optional: only these things, each a network, the AP it is joined on and a band (2g, 5g, 6g, or wire for a test over the wire, on the AP that makes it), as get_assurance's checks say them: what failed, asked again (0129)"`
}

type alertsIn struct {
	Under string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only the APs below it"`
}

type libraryIn struct {
	At string `json:"at,omitempty" jsonschema:"a Locations node: list the templates offered there"`
}

type adviceIn struct {
	Scope string `json:"scope,omitempty" jsonschema:"a Locations folder with an advisor of its own: its findings and reviews; the first you may view unless set"`
}

type nameIn struct {
	MAC  string `json:"mac" jsonschema:"the client's MAC"`
	Name string `json:"name" jsonschema:"what to call it, at most 64 characters on one line, as kettle or garage AC; empty takes its name away"`
}

type findingIn struct {
	ID    int64  `json:"id" jsonschema:"the finding's number, as get_advice lists it"`
	State string `json:"state" jsonschema:"resolved (its problem is gone: found again, it opens again), dismissed (looked at, not to act on: found again, it stays dismissed unless worse) or open"`
	Why   string `json:"why,omitempty" jsonschema:"for resolved: what fixed it, or what shows it gone, with the evidence, at most 2000 characters"`
}

type keysIn struct {
	Folder  string `json:"folder" jsonschema:"the Services folder that offers the network"`
	Network string `json:"network" jsonschema:"the network's ID"`
}

type treeIn struct {
	Tree string `json:"tree" jsonschema:"locations or services"`
}

type nodeIn struct {
	Tree string `json:"tree" jsonschema:"locations or services"`
	Node string `json:"node" jsonschema:"node ID"`
}

type apIn struct {
	AP string `json:"ap" jsonschema:"AP ID in the Locations tree"`
}

type changesIn struct {
	After int64 `json:"after,omitempty" jsonschema:"return changes after this sequence number"`
	Limit int   `json:"limit,omitempty" jsonschema:"how many changes to return, 1 to 1000 (default 100)"`
}

type previewIn struct {
	Op Op `json:"op" jsonschema:"the change to preview"`
}

type actionIn struct {
	AP       string   `json:"ap" jsonschema:"AP ID in the Locations tree"`
	Kind     string   `json:"kind,omitempty" jsonschema:"locate, restart-wifi, reboot, disconnect, client-test, remove-network or import-network; empty to list its latest actions"`
	Target   string   `json:"target,omitempty" jsonschema:"for disconnect: the client's MAC"`
	Network  string   `json:"network,omitempty" jsonschema:"for client-test: the network to join, by its ID, one the AP offers; for import-network: the ID the new network is to have"`
	On       string   `json:"on,omitempty" jsonschema:"for client-test: the AP whose networks to join, the testing AP itself among them; empty for the strongest other AP it hears"`
	Band     string   `json:"band,omitempty" jsonschema:"for client-test: 2g, 5g or 6g; empty for any; wire for a test over the wire, from the AP itself"`
	Remote   *bool    `json:"remote,omitempty" jsonschema:"for client-test: false to leave out the speed to the remote server, which moves a good deal over the internet link; measured where one is set unless false"`
	SSID     string   `json:"ssid,omitempty" jsonschema:"for remove-network and import-network: the SSID of the AP's own network"`
	Sections []string `json:"sections,omitempty" jsonschema:"for remove-network and import-network: the AP's names for that network in its config, one a radio, as its state report's others has them"`
	Folder   string   `json:"folder,omitempty" jsonschema:"for import-network: the Services folder the network goes in"`
	Bands    []string `json:"bands,omitempty" jsonschema:"for import-network: the bands it is to be on, 2g, 5g, 6g; empty for every band"`
	VLAN     int      `json:"vlan,omitempty" jsonschema:"for import-network: the VLAN its clients are on"`
}

type changeIn struct {
	Op     Op     `json:"op" jsonschema:"the change to make"`
	Reason string `json:"reason,omitempty" jsonschema:"optional: a note for the change log, which records who made the change and when without one"`
}

type empty struct{}

func server(c client, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "aeolus", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "whoami", Description: "Your account and the roles you hold on folders.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/whoami", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_suggestions", Description: "What people asked Aeolus to do, from its pages (0323), newest first: what they wrote, how to reach them if they said, the page and release they asked from, who asked, and whether it has been sent on (sent_at, sent_to) or why not yet (last_error). to is where they go on to; empty where they are kept on this manager. For those who view the Org.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/suggestions", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_tree", Description: "List the nodes of the locations or services tree that you can view, root first.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in treeIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/trees/"+url.PathEscape(in.Tree), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_node", Description: "One node's page: every field with its value, where it comes from (from, origin), the overrides in effect and below, the locks a break would escape, and the problems (rules its config breaks).", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nodeIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/trees/"+url.PathEscape(in.Tree)+"/nodes/"+url.PathEscape(in.Node), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_ap_config", Description: "An AP's fully resolved config: Location fields, networks from its service folders, its version, and whether the whole config passes its check. Its condition says when it was last seen and from where, the version it last reported running and whether that is current (in_sync), and its latest render check, apply and state report.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in apIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/aps/"+url.PathEscape(in.AP)+"/config", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_ap_history", Description: "An AP's recent render checks (with their problems), apply attempts (and whether a passing check covered them) and state reports, newest first.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in apIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/aps/"+url.PathEscape(in.AP)+"/history", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_library", Description: "The Org's library: AP templates (0085). A template is a named set of Locations settings for the APs of one or more boards (arista,c-360), made at the Org or a folder and offered there and below. A folder picks one for a board with templates.<board>; an AP takes the one picked nearest above it, its values counting as set where it is picked, ahead of that folder's own, so what is set below or locked replaces them. With at, the templates offered at that Locations node, nearest first. Each lists the APs that take it and whether each follows it; an AP's config (get_ap_config) says which of its template's fields something else replaces. Tunnels are not in the library: they are Location fields, concentrators.<name> (0055).", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in libraryIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/library"
			if in.At != "" {
				path += "?at=" + url.QueryEscape(in.At)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_relayed_dhcp", Description: "What the manager hears from DHCP relays that copy it clients' requests (0068): for each subnet, by the relay's address on it, the relay, the requests of the last 10 minutes, the clients new in that time, a burst of new ones (a sign of DHCP starvation), and its clients newest first, each with its host name, vendor class, parameter list, address, option 82 IDs, maker and the manager's guess at what it is. Relays copy only requests, not servers' answers.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/dhcp/relayed", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_alerts", Description: "What needs attention on the APs you can view (0099), most urgent first, with counts by severity; an AP in maintenance (0218, maintenance.on with until and why at a Locations node; with maintenance.hold too, a test window, 0344, in which the AP holds still: no RRM channel or power moves, no steering, no agent update, no client test on a schedule, until it ends by the AP's own clock) has its alerts last, each with maintenance saying so, counted apart as maintenance and never sent: critical (clients are or will be without service: an AP offline, a config refused or put back, a network with no transport, a loop, netifd's lost wireless object, a radio with none of its networks on the air for 4 minutes or more, 0216; a radio whose firmware crashed and did not come back, fw-crash, 0310, two minutes on or said lost), warning (a radio whose firmware crashed and came back, in the last 6 hours, or is starting again, fw-crash, 0310; a config held, a radio with some of its networks not on the air for 4 minutes or more (0216), a client the APs served heard sending to a rogue in the last 15 minutes (client-on-rogue, 0160), an agent update rolled back, a tunnel down, a network on its fallback, a VLAN silent on the uplink, DHCP that does not answer, an AP behind its version, a network that failed its last two client tests on an AP, a client that came online at an AP 20 times or more in the last hour and drops its connection itself, rejoins, 0190: it blames the AP's radio only where the client stayed on another AP in that hour, and the client or what is beyond the APs where it did the same on the others, 0200, and the weaker link where the AP it stayed on heard it 10 dB or more more strongly and this one at -75 dBm or under, 0312, a client a person named that struggles over two hours, on the AP it struggles on (0306), with where it did better in the last two days, if it did (0309), struggling, 0233: four moves or more between that AP's bands, 30% or more of the frames to it sent again, or slow for its signal on 5 or 6 GHz, under 120 Mbit/s at -60 dBm or better and a bar 6.5 lower a dB below that to -75, 0242 and 0250; a radio that runs slowly, slow-radio, 0242: its last three client tests that measured speed each got under a third of their tester's usual from it, down or up, and no other tester got its usual from it since (0308); state reports the manager refused whole in the last 15 minutes, reports-refused, or took without elements of a list it could not read, report-trimmed, keyed by the list, 0290; an AP that calls, has been up 10 minutes and serves networks, whose radios sent no minute for 10 minutes, or one serving radio that sent none while the others' come, minutes-stopped, 0311; an AP that calls whose state reports stopped over 15 minutes ago, reports-stopped, 0315; a client pinned to it heard there far weaker than its usual over the last half hour, moved perhaps, pin-moved, 0340; a library some process on the AP holds two builds of at once, mixed-libs, keyed by the library, 0317) and info (waiting in Landing Zone, a clock not synced). With under, only the APs below that Locations node. Worked out from what the manager has on each call, so an alert ends when its cause does.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in alertsIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/alerts"
			if in.Under != "" {
				path += "?under=" + url.QueryEscape(in.Under)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_assurance", Description: "Service assurance (0123): the client tests (0121) of the APs you can view that can test, through the air with a scan radio or over the wire (0128), over the last hours (24 unless set, at most 720). testers: each such AP, how often it is asked for a test on a schedule (every, in minutes; 0 for never; set with make_change on assurance.tests.every at a folder, assurance.tests.speed for a measurement against the local speed server in them and assurance.tests.speed_remote for one against the remote server (0134), and assurance.tests.over for which tests, 0131: air, through the air by an AP with a scan radio, which it is unless set; wire, over the wire by every AP below, a scan radio or none, each of its networks, 0128; or both. A tester says which in over, has wire true where it is asked for tests over the wire and can make them, and what it tests over the wire has wire for its band) and how many things it tests in turn. checks: each thing tested, a network on an AP on a band, by which AP, with how many of its tests passed and failed, its latest, and whether it is failing (its last two failed, which is also an alert on the AP it failed on), the failing first; way is what its tests' ways online come to, a point at a time (0130): at each point how many passed it, how many stopped there, and the middle (ms), least and most of their times in ms. A test's own detail.way is its points in order, each ok, bad, begun or none, with how long it took. tests: the latest 50, newest first, each as ap_action lists it. round: a round of every test asked by hand that the APs are in (ask_assurance_round, 0129), with which tests it is of (over: air, wire or both, 0131), how many things that is, and how many were asked for and are done since. joins: how the clients' own attempts to come online went over the same time (0127), a network each, on every AP you can view, scan radio or none: how many clients tried, got on (online) and did not (failed), each counted by the latest of its attempts to come to anything, how long one typically took to come online (typical_ms: until the network had let it on and given its address, 0322; not until it first asked for anything, which an IoT device may not do for seconds), where the failed attempts stopped and why (stops, the most clients first), each AP's part (aps), the attempts' way online a point at a time as a check's is (way, over way_of attempts), and failing where three clients or more were let on at an AP in the last hour and got no further (no address, no gateway, no DNS), being half or more of those let on there, which is the alert joins-failing on that AP. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/assurance?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_roaming", Description: "Roaming (0136): the clients that moved from one AP to another, or from one radio of an AP to another, on the APs you can view, over the last hours (24 unless set, at most 720); with under, the APs below that Locations node. A roam is a client's attempt to come online at an AP other than the one it was last let on at, where it came as one already on the network (a reassociation) or by a fast transition, or where it was on that other, on the same network, minutes before; kind full is a hard roam, the sign-in or WPA3's exchange again. A move from one radio of an AP to another of the same is a band switch (band_switch on its row), counted in band_switches and in no other count. roams, failed (not let on), slow (let on after more than slow_over_ms, 250), left (given up by the client before it was let on, not counted as roams), clients, untimed (let on, and not timed: their AP's log has the association and no line of the authentication, as on a network that signs clients in), and typical_ms, the middle of the times of the rest, from a client's authentication at the AP it came to, as its log has it, until it was let on. kinds: the same a kind at a time, with low_ms and high_ms: ft (802.11r, the keys come with it), okc (a key made from its sign-in at another AP), pmksa (a key the AP held from its last time there), cached (one of those two, on a network that signs clients in, where the AP does not say which), keys (a passphrase: the four key messages) and full (the sign-in or WPA3's exchange again). latest: the latest 50, each {id, at, mac, host, ap, ap_name, network, ssid, band, from_ap, from_ap_name, from_band, kind, ms, untimed, state, reason, sa_queries}, state being ok, slow, failed or left, from where the client was last let on before, on any AP, where that is known and yours to view, and sa_queries how often the AP asked a client it still held to prove itself before taking it (802.11w), a second's wait. bad: those of them that are not ok. id is the attempt's, as get_client_connections lists it. key_waits (0231): each AP's wait, a band at a time, between a client's last key message and its being let on, {ap, ap_name, band, joins, typical_ms, high_ms}, over every join of the time with the key messages: the radio storing the key, 1 or 2 ms where nothing holds it up; 20 or more means the AP's kernel or driver holds every join up (B94, mac80211, 30-60 ms).", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/roaming?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_compatibility", Description: "How each maker's Wi-Fi clients fare on each model of AP, a band at a time (0193), on the APs you can view, over the last hours (24 unless set, at most 720); with under, the APs below that Locations node. groups: each {maker (from the MAC's OUI; a private address, or unknown), model (the AP's), band, clients, attempts, online, failed, per_hour (how many times an hour the middle client came online again at an AP while it was there: 0 for one that stayed; counted over an hour at least where a client came online fewer than 20 times there, so one rejoin is not a loop), looping (clients that came online again 20 times an hour or more, as one that drops its connection itself, 0190), aps, worst (up to 5 clients, the most often first: mac, host, ap, ap_name, joins, per_hour), trouble, everywhere}. trouble is said where a maker's clients loop on one model and band, with where, on other models, they stay: a device that does not get on with a kind of radio, not one client's bad day. Where they loop on other models too, it is everywhere instead, and no trouble: the looping is no model's, and points at the clients or the path beyond the APs (0200). The troubled come first. The APs' own test clients are left out.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/compatibility?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_network", Description: "How long the networks' own services took to answer the APs you can view (0141), over the last hours (24 unless set, at most 8760): for each VLAN or tunnel the networks are on, and for all together, how many times each step was timed, its average and its slowest in ms, and how many times no answer came. The steps are dhcp (an address, from asking to having one), gateway and dns, as client tests found them, aaa (the RADIUS server's answer to the AP, from each state report) and signin (a whole 802.1X sign-in). With series, each step's average over the time in at most 48 buckets of whole hours. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/network?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_steering", Description: "Whether steering works (0139): what the steering of the APs you can view asked of clients and what came of it (0138), over the last hours (24 unless set, at most 720); with under, the APs below that Locations node. The manager keeps each asking an AP's steering says in its state reports, once, so the counts are of the time asked for and not since an AP's steering last started. networks: each {network, counts, aps}, counts being for each way (suggest: a client asked to move to a higher band by 802.11v; hold: one kept off 2.4 GHz a moment as it came to join; sticky: a client told to go, weak or heard more strongly by another AP by its probes, or saying it hears another AP more strongly, 802.11k (0289)) how many were asked and how many came to each end: moved (it went to a better band here), refused (it said no), stayed, left (it went elsewhere), joined (held off, it joined 2.4 GHz all the same), recovered (a weak client was heard well again), back (moved up, it came back down within five minutes; or, put off by sticky, it came back within ten minutes, as a record of its own where its asking had ended as left, 0289). aps: the same an AP at a time, each {ap, name, counts}. clients: the 20 asked most, each {mac, host, asked, moved, refused, back, last}. latest: the latest 100 askings, each {at, ap, ap_name, mac, host, network, method, from, to, asked, answer, code, signal, outcome, took, why, elsewhere}: from the band it was on, to the bands it was pointed at, asked btm (a transition request), held, kick or none, answer accepted or refused with the client's 802.11v status code, signal in dBm, took in seconds; for sticky, why (weak; better: another AP heard it more strongly, or it said it hears another AP more strongly; home: it was not where a person said it belongs, homes.<mac> at a Locations folder, 0319; or nearest: another AP heard it 5 dB or more better and its rule, homes.<mac>.nearest, sends it to the AP that hears it best, 0321) and elsewhere, that AP {ap, name, signal} where you may view it, signal only where it was heard there; after (0291), where the client was let on next within five minutes of the asking's end {ap, ap_name (where you may view it), band, signal, at}, looked for only on an asking (not a came-back record that asks nothing) that ended moved, left, came back or joined, and result, what that came to: back (where it was asked from), band (on another band, not compared), stronger or weaker (heard 3 dB or more better or worse there, on the same band), alike, or unknown (a signal not known). networks and their aps gain results, those counted over the latest askings weighed (at most 300). The tests' own clients are left out.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/steering?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_minutes", Description: "What each minute came to on the APs' radios, and for a client (0207): every AP samples its radios and their clients every 5 seconds and says at each minute's end what the minute came to; the manager keeps the radios' minutes 30 days and the clients' 7. Of the APs you can view; with under, those below that Locations node; with band, that band's radios alone. radios: each {ap, ap_name, radio, band, minutes}, the oldest first over the last hours (1 unless set, at most 24), each minute {at (its start), channel, width, power (dBm), busy (percent of the minute the channel was busy, others' use and the radio's own), busy_max (the busiest 5 s), rx and tx (percent of the minute it received and sent), noise (dBm), clients (on average) and clients_max, tx_packets, tx_retries and tx_failed (what was sent to its clients, tried again, and lost), rx_packets, tx_bytes and rx_bytes}. With mac, client: {mac, host, minutes} in place of radios, over as many as 168 hours, each {at, ap, ap_name, band, ssid, signal and signal_min (dBm, average and weakest), tx_rate and rx_rate (Mbit/s), the same counts, secs (how much of the minute it was there)}. For finding what a channel's load, a client's signal and its retries did over time, beside what happened.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in minutesIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Band != "" {
				q.Set("band", in.Band)
			}
			if in.MAC != "" {
				q.Set("mac", in.MAC)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/minutes?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_recordings", Description: "The flight recorder's recordings (0207 part 3): when something went wrong on an AP (on a radio, what was sent to its clients tried again or lost all at once, many of its clients gone at once, or its channel pinned full; power control's clients taking a step down badly; or someone asked), the AP sent what each five seconds came to from ten minutes before to ten after. Of the APs you can view; with under, those below that Locations node; over the last hours (168 unless set, at most 720). recordings: each {id, ap, ap_name, at (when it went wrong), why (retries, clients-left, busy, distress, asked, radios-down: every radio down at once, or radios-restart: they came back, said on new phys where the Wi-Fi driver was loaded again, else the Wi-Fi restarted), said (what the AP said of it), from, until, points}, the latest first. Read one with get_recording.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/recordings?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_recording", Description: "One of the flight recorder's recordings (0207 part 3), with its points: recording {id, ap, ap_name, at, why, said, from, until, points}, and points, the oldest first, each five seconds {at, radios, clients}: each radio {radio, band, channel, width, power, busy, busy_max, rx, tx, noise, clients, tx_packets, tx_retries, tx_failed, rx_packets, tx_bytes, rx_bytes}, busy and rx and tx in percent of the five seconds; each client, the 24 moving the most data, {mac, host, band, ssid, signal, tx_rate, rx_rate, heard (what it says it hears of the AP, 802.11k, where it can), the same counts, secs}. For seeing what led up to something going wrong, and what came of it.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in recordingIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", fmt.Sprintf("/v1/recordings/%d", in.ID), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_power", Description: "Power control (0077, 0206) on the APs you can view; with under, the APs below that Locations node. aps: each {ap, name, on, clients, client_floor, hold, reported, radios}: clients is what power control does for a client no other AP hears better (guard: it keeps the AP loud enough that such a client hears it at client_floor dBm, and steps back up and holds hold hours when its clients take a step down badly; ignore: it steps by its neighbours alone; ask, the default: it steps by its neighbours and the manager warns). radios: each as the AP last said {radio, band, power, ceiling, neighbours, target, count, weakest, step, why, ago, floor (the power its lonely clients need), hold {power, left seconds}, lonely (the loneliest first, at most 8: mac, host, signal as the AP hears it, est what it hears of the AP, need the power for it to hear the AP at client_floor, other the best another AP hears it), distress (its clients' last bad turn after a step down: at, why scanning, leaving or retries, scanning and left counts, ratio of retried or lost before and after, acted whether it stepped back)}. steps: power control's steps over the last hours (24 unless set, at most 720), the latest first, at most 500, each {ap, ap_name, at, band, from, to, why, scanning, left, ratio}.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/power?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "ask_assurance_round", Description: "Ask every AP below a Locations folder, or one AP, for a round of client tests now (0129): each tests every thing it can, once, one at a time from its next poll, whatever its schedule or none: through the air where it has a scan radio (0121), and over the wire where its agent can (0128), whether a folder asks for that on a schedule or not. over says which tests, 0131: air, wire, or both, which it is unless said. For checking a site that was just built or changed; networks, bands and on narrow it to a troublesome few (0188): only those networks, only those bands through the air, only what joins on those APs. It needs operator there, and is refused where no AP there can test. A round takes about a minute a thing for each AP; what is not asked for within the hour is let go. Read how it goes with get_assurance: round says how many things, asked and done, and checks and tests what they found.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in roundIn) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"under": in.Under}
			if in.Over != "" {
				body["over"] = in.Over
			}
			if len(in.Networks) > 0 {
				body["networks"] = in.Networks
			}
			if len(in.Bands) > 0 {
				body["bands"] = in.Bands
			}
			if len(in.On) > 0 {
				body["on"] = in.On
			}
			if len(in.Only) > 0 {
				body["only"] = in.Only
			}
			out, err := c.call(ctx, "POST", "/v1/assurance/round", body)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_alert_history", Description: "The alerts that lasted on the APs you can view (0109), at some time in the last hours (24 unless set, at most 720), newest first, at most 500: each with its AP, severity, kind, key and message, when it began, and when it ended, or none while it lasts. An alert is logged once it has lasted two minutes, from when it was first seen; a shorter blip is not. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/alerts/history?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_airspace", Description: "The Wi-Fi networks the APs you can view hear (0106), once each by BSSID, with its SSID, band and channel, and every AP that hears it with its signal and when it last did, the strongest first. Each is a rogue (it broadcasts one of Aeolus's SSIDs from a BSSID no AP names as its own: an evil twin, or a same-named AP Aeolus does not manage), known (such a BSSID a folder's rogues.known says is no rogue; set it with make_change to quiet one), aeolus (one of the APs' own, heard by an AP that is not its radio neighbour) or other; rogues first. Only APs with radio resource management on listen. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in alertsIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/airspace"
			if in.Under != "" {
				path += "?under=" + url.QueryEscape(in.Under)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_threats", Description: "The threats the APs you can view find (0160). rogues: the rogue APs, suspects and known ones the APs hear now, once each by BSSID, with its maker by the BSSID's prefix (maker; maker_derived where taken from a sibling BSSID heard with it, or its own with the locally administered bit cleared), its SSID, band and channel, the APs that hear it, the strongest first, and wire: what the scan radios' wire check found (on: frames sent on your network came out of its radio, with the VLANs; off: they looked, looks times, and never saw that; unchecked: no scan radio hears it). Its kind is rogue (not one of the APs' and on your wire: in your network for certain), suspect (one of Aeolus's SSIDs, not seen on your wire: an evil twin, a neighbour's AP of that name, or one of yours out of reach) or known (marked known by a folder's rogues.known; set it with make_change). clients: the clients the scan radios heard on a rogue or suspect over the last hours (24 unless set, at most 2160), the last heard first: MAC, the rogue's BSSID, SSID and kind now, first and last heard, frames, the APs that heard it, whether it is one the APs have served (ours, with its host name) and the networks that block it. dhcp: the networks on which more than one DHCP server answered in the last hour, or, where a network lists the servers it has (network.<id>.dhcp_servers, by address or MAC), one that is not among them; each server with its address, MAC, answers, when it last answered, the APs that saw it, and known where it is one the network lists. Only APs with a scan radio check the wire and hear clients. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/threats?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_usage", Description: "The Wi-Fi clients and traffic of the APs you can view (0108), over the last hours (24 unless set, at most 8760, a year; over more than two days from the trends kept by the hour, 0140), in at most 48 buckets: in each, the most clients the APs had at once, summed over the APs, and the bytes they sent their clients (down) and received from them (up); the totals; each AP's most clients and traffic, the busiest first; and the ten clients that moved the most (0110), by MAC and host name, with the APs they used. Worked out from the APs' state reports, every few minutes. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/usage?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_client_journey", Description: "One Wi-Fi client's history across the APs you can view (0103), by its MAC, over the last hours (24 unless set, at most 720): its sessions, each on one AP, band and network, from and to, with its signal (min, max, mean), mean rate, retries (the share sent again over its busy minutes alone, those it moved a megabyte or more in, and busy_minutes, how many; none where it had none, or its minutes, kept 7 days, are gone: 0303), address and DHCP verdict; how often it roamed; and issues: a weak signal, many retries (over a fifth, over 3 busy minutes or more), a slow rate, no DHCP or a static address, moving back and forth between two APs, or coming online at one AP 20 times or more within an hour (rejoins, 0190: it drops its connection itself; read from the APs' records of its attempts, and where it is, moving back and forth is not said; it blames that AP's radio only where the client stayed on another AP, and the client or what is beyond the APs where it did the same on the others, 0200), or dropping off and coming back 4 times or more within an hour on whichever APs, away 10 s or more each time (drops, 0259: from why it left, 0255, and where it went next, 0258; a client moving between rooms is back at once, so is not one). Built from the APs' state reports, every few minutes, so a session's edges are to within a report.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in journeyIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/clients/" + url.PathEscape(in.MAC)
			if in.Hours > 0 {
				path += "?hours=" + fmt.Sprint(in.Hours)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "trace_client", Description: "Live Trace (0144): one Wi-Fi client, on or not, watched as it moves from AP to AP. With do unset it reads the trace: whether it is traced, by whom and until when, how many APs you can view and which of them hold their line to the manager (0145: those hear of a trace at once, and each is listed with how far its clock is from the manager's), and what the APs said of the client, oldest first as the manager took them, at most 500 a read: each with its time by the manager's clock, the AP, the network and band, the stage (auth, assoc, signin, key, dhcp, gateway, dns, internet, end; heard, which is not on and heard asking; signal, which is on and how strongly; done, what an attempt came to), what in words, whether it went well, the signal in dBm, and lag_ms, how long after it happened the manager had it. Sort by the time to read a move in order; pass next back as after to read only what came since. With do: start it asks every AP to trace the client for minutes (15 unless set, at most 60), or to go on that long from now; four clients at once. With do: stop it ends the trace, which can still be read for a day. Starting and stopping need the operator role on an AP. Kept in the manager's memory: a restart of the manager ends every trace."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in traceIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/clients/" + url.PathEscape(in.MAC) + "/trace"
			switch in.Do {
			case "":
				if in.After > 0 {
					path += "?after=" + fmt.Sprint(in.After)
				}
				out, err := c.call(ctx, "GET", path, nil)
				return nil, out, err
			case "start":
				if in.Minutes > 0 {
					path += "?minutes=" + fmt.Sprint(in.Minutes)
				}
				out, err := c.call(ctx, "POST", path, nil)
				return nil, out, err
			case "stop":
				out, err := c.call(ctx, "DELETE", path, nil)
				return nil, out, err
			}
			return nil, nil, fmt.Errorf("do: start, stop, or unset to read")
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_clients", Description: "Every Wi-Fi client ever seen whose last AP you can view (0118), the latest first, kept for good: each one's MAC, when it was first and last seen and on which AP and network, the host name it gives, who it signed in as, its address, how many times it tried to come online and how many of those failed, and how its latest attempt went. Find one by part of its MAC, host name, user or address.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in clientsIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Find != "" {
				q.Set("q", in.Find)
			}
			if in.Limit > 0 {
				q.Set("limit", fmt.Sprint(in.Limit))
			}
			if in.Offset > 0 {
				q.Set("offset", fmt.Sprint(in.Offset))
			}
			out, err := c.call(ctx, "GET", "/v1/clients?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_leaving", Description: "Why clients left (0255), on the APs you can view, over the last hours (24 unless set, at most 2160), a band at a time (never added together); with under, the APs below that Locations node; with mac, one client; with band, one band. Each leaving is named by who sent it off and with what reason, as the AP's hostapd frames said: leaving (the client said so), client-other (the client left saying something was wrong), idle, unheard (too many frames went unacknowledged: the client gone or out of reach), keys, signin, busy, state, sent-off (the AP sent it off, by its reason), network (the AP's frame to every station: the network went down or started anew), moved (Aeolus let go of it as it joined another AP, 0226), asked (a person disconnected it, 0107), steered (steering put it off, or a BSS transition), off-air (its network was taken off the air), silent (no frame from it or the AP: gone with no word). trouble marks the kinds worth a look. bands: each band's total and its kinds with counts, the most first; latest: the latest 50, each {at, ap, ap_name, mac, band, ssid, kind, words, trouble, by, frame, reason, reason_words, signal, last, inactive_ms, inactive_ago_s, next}, signal how strongly the AP last heard it, last its last minute there (tx_packets, tx_retries, tx_failed, rx_packets), inactive_ms how long it had sent nothing by the AP's latest report, and next where it went after (0258): its next attempt {ap, ap_name, after_s, outcome} on any AP within 10 minutes, after_s below 0 where it was joining another AP already; none where it made no attempt, as a client switched off. dropping: the clients that kept dropping (0259), the most first, each {mac, times, first, every_s, away_s, aps}: 4 times or more within an hour, on whichever APs, each a leaving that was not Aeolus's doing nor its network's, after which it tried to join again 10 s to 10 minutes later. The APs' own test clients are left out. Only leavings an agent of v0.206.0 or later said are here.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in leavingIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			for k, v := range map[string]string{"under": in.Under, "mac": in.MAC, "band": in.Band} {
				if v != "" {
					q.Set(k, v)
				}
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/leaving?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_client_connections", Description: "One Wi-Fi client's attempts to come online, on the APs you can view (0118), by its MAC, the latest first, kept 90 days: each with its AP, network and band, when it began, how it went (online, connected, failed or left), the stage it reached or stopped at (auth, assoc, signin, key, dhcp, gateway, dns, internet) and why, its way online a point at a time (way, 0130: each point ok, bad, begun or none, and how long it took in ms; the keys with parts, the four messages of their handshake m1 to m4, where the AP said them, 0133: a wrong passphrase stops at m2 with said \"wrong passphrase\", a client that does not answer with \"no answer\"), and its record: every step the AP saw, with its time in ms after the start. The steps are hostapd's own words for authentication, association, an 802.1X sign-in and the keys; DHCP's messages with the server, gateway and DNS servers given; the ARP for the gateway and its answer; the first DNS lookups and answers; and the first TCP connection opened and answered. Also where the client was heard asking for networks before it joined, where band steering runs. A failed attempt is here though the client never appears in a state report.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in connectionsIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Limit > 0 {
				q.Set("limit", fmt.Sprint(in.Limit))
			}
			if in.Before != "" {
				q.Set("before", in.Before)
			}
			out, err := c.call(ctx, "GET", "/v1/clients/"+url.PathEscape(in.MAC)+"/connections?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_detected", Description: "Devices that may be unconfigured OpenWiFi APs (0034, 0068): possible when a relayed DHCP request asks for options 138 and 224, confirmed when the device knocked on the manager's option 224 listener; and the knocks no device could be matched to by address.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/detected", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_keys", Description: "A network's per-user keys (0070): each its own passphrase on a shared WPA2-PSK network, and the VLAN its client lands in. Lists each key's ID, name, VLAN, the MACs it is bound to and its expiry; passphrases are sealed and never shown. Keys are made, changed and removed with make_change (add-key, set-key, remove-key), and re-version no AP.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in keysIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{"folder": {in.Folder}, "network": {in.Network}}
			out, err := c.call(ctx, "GET", "/v1/keys?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_advice", Description: "What a folder's advisor found (0205, 0212): each Locations folder that sets system.advisor.key itself has an advisor, which reviews that folder and what is below it (but a folder below with a key of its own), billed to that key. In its reviews, Claude reads the manager as the account aeolus-advisor, held to that folder, and records problems with clients, APs, networks and settings, each with what it saw and what to do, and resolves those it finds gone; a review may answer one question a person asked instead (0215). scopes: the advisors you may view, each {scope, name, enabled, model, every, web, budget, running, open, may_review, may_dismiss}; scope: the one shown. findings: its, the open first, each {id, scope, key, kind (client, compatibility, ap, network, config), severity, title, saw, todo, subjects (clients, aps, networks), state (open, dismissed, or resolved: its problem gone), state_by, state_why (for resolved: what was seen), times found, first_at, at}. runs: its latest reviews, with the question each answered where it was asked one (its summary is then the answer), their model, requests, tokens, cost in US dollars, outcome (done, budget, failed) and summary or error. account_set: whether the advisor's account is set up.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in adviceIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/advisor"
			if in.Scope != "" {
				path += "?" + url.Values{"scope": {in.Scope}}.Encode()
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "name_client", Description: "Give a client a name of your own (0153), shown for it wherever the manager names it: its pages, the clients lists and its alerts, its MAC after. An empty name takes it away. The client must have been seen on an AP you can view, and you need the clients right there. Nothing is told to an AP, and it is not in the change log.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: ptr(false)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nameIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "PUT", "/v1/clients/"+url.PathEscape(in.MAC)+"/name", map[string]any{"name": in.Name})
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "set_finding", Description: "Say a finding of the advisor's (0205) is resolved, dismissed, or open again (0215): resolved where its problem is gone, with why (what fixed it, or what shows it gone); dismissed where it was looked at and is not to act on. It needs operator at the folder whose advisor found it. It changes nothing on the network.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: ptr(false)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in findingIn) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"state": in.State}
			if in.Why != "" {
				body["why"] = in.Why
			}
			out, err := c.call(ctx, "POST", fmt.Sprintf("/v1/advisor/findings/%d", in.ID), body)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_changes", Description: "Read the change log: who changed what, when and why.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in changesIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.After > 0 {
				q.Set("after", fmt.Sprint(in.After))
			}
			if in.Limit > 0 {
				q.Set("limit", fmt.Sprint(in.Limit))
			}
			path := "/v1/changes"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "preview_change", Description: "Run a change with every check (permissions, schema, locks, APs that would stop resolving) and record nothing. Returns the effect, overrides a lock or move would remove, the APs it would re-version, and any whose config would then fail its check.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in previewIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "POST", "/v1/preview", map[string]any{"op": in.Op})
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "ap_action", Description: "Ask an AP to do something once (0104): locate (blink every LED for a minute, to find it), restart-wifi (every client drops and joins again), reboot (off the air for about two minutes) or disconnect (0107: one client, by MAC in target, leaves every network it is on there and may join again at once, to make it reconnect); or client-test (0121: an AP with a scan radio joins one of its own networks, named in network, as a client would, on the AP named in on or else the strongest other AP it hears, and times each step: finding it, the keys, DHCP, the gateway, DNS and, where a speed server is set, how fast, to the local one (system.speed_server) and to the remote one (system.speed_server_remote, 0134: a public server by its name, as cloudflare, or one's own by its URL; with remote false that one is left out, as on a link where bytes cost); its result is the action's detail, read by listing the AP's actions a minute or two later; with band wire it is made over the wire instead (0128), by any AP whose agent can, a scan radio or none: from its own uplink, on the network's VLAN or into its tunnel, it asks for an address by DHCP, its gateway by ARP and a name by DNS, joins nothing, uses no radio and takes a few seconds); or remove-network (0137: the AP takes one of its own Wi-Fi networks, set on the AP and not by Aeolus, out of its config, named by ssid and by sections, its names for it, one a radio, as the others of its state report has them; its clients are off it for good, the AP keeps what it takes out in /etc/aeolus/removed, and Aeolus has no copy: ask the person before this one); or import-network (0137: makes one of the AP's own networks Aeolus's, in the Services folder named in folder, under the ID in network, on bands and vlan: the AP says the network as it has it, its passphrase with it, on its next poll, and the manager makes the network then, in one change; each AP that has the same network of its own, with the same passphrase, takes its own out as Aeolus's goes in; the action's result says which change it was and what was not carried over). It needs operator on the AP. The AP takes it up on its next poll, within about a minute; one it does not take up within 10 minutes expires. A second ask of the same kind while one waits is the same action. Without kind, lists the AP's latest actions and what came of each.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in actionIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/aps/" + url.PathEscape(in.AP) + "/actions"
			if in.Kind == "" {
				out, err := c.call(ctx, "GET", path, nil)
				return nil, out, err
			}
			body := map[string]any{"kind": in.Kind}
			if in.Target != "" {
				body["target"] = in.Target
			}
			for k, v := range map[string]string{"network": in.Network, "on": in.On, "band": in.Band} {
				if v != "" {
					body[k] = v
				}
			}
			if in.Remote != nil {
				body["remote"] = *in.Remote
			}
			if in.SSID != "" || len(in.Sections) > 0 {
				body["ssid"], body["sections"] = in.SSID, in.Sections
			}
			if in.Folder != "" {
				body["folder"] = in.Folder
			}
			if len(in.Bands) > 0 {
				body["bands"] = in.Bands
			}
			if in.VLAN != 0 {
				body["vlan"] = in.VLAN
			}
			out, err := c.call(ctx, "POST", path, body)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_ends", Description: "The changes that end themselves (0345) and have not ended yet, the soonest first: each {seq, at, actor, reason, until, steps}, steps being what its end puts back, {tree, node, path} for a field (put back as it was, unless something changed it since) or {tree, node, folder: true} for a folder it made, taken away. The manager ends each at its until, in its own name; end_change ends one sooner or moves its end. Needs viewer at the Org root.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/ends", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "end_change", Description: "End a change that ends itself (0345) now, or with until move its end: now, it puts back what the change did wherever nothing has changed it since, takes away the folders it made, and says what it left; logged as end-change under your name. Moving it may set fields of one Locations node with the move, as part of the change. Either needs the right to make the change again."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in endIn) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"reason": strings.TrimSpace(in.Reason)}
			if in.Until != "" {
				body["until"] = in.Until
			}
			if len(in.Values) > 0 {
				body["tree"], body["node"], body["values"] = "locations", in.Node, in.Values
			}
			out, err := c.call(ctx, "POST", fmt.Sprintf("/v1/ends/%d", in.Change), body)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "make_change", Description: "Make one change. It is logged under your name with the time, and with your reason if you give one. Set values are checked against the field schema and secrets are sealed. Returns the logged change, the APs it re-versioned, and any whose config now fails its check.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in changeIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "POST", "/v1/changes", map[string]any{"op": in.Op, "reason": strings.TrimSpace(in.Reason)})
			return nil, out, err
		})
	return s
}

func ptr[T any](v T) *T { return &v }
