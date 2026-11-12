// Package advisor is the manager's advisor (0205): a review in which Claude
// reads what the manager knows, as the account aeolus-advisor, a viewer,
// and records the problems it finds, each with what to do, and resolves
// those it finds gone (0215). A review may answer one question a person
// asks instead of looking over everything. It changes nothing on the
// network: its tools are the MCP adapter's reads, and its writes are
// findings in the conditions store, for people to read.
package advisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

// Account is the account a review reads as: a viewer of both trees, set up
// by an owner, whose roles say what it may read (0205).
const Account = "aeolus-advisor"

const (
	// maxRequests is the most requests a review makes.
	maxRequests = 40
	// maxOut is the most an answer may be, in tokens.
	maxOut = 16000
	// taskBudget is the tokens a review is told it has, so it paces itself.
	taskBudget = 200000
	// searches is the most web searches one request may make.
	searches = 3
	// findingsShown is how many findings so far a review is told of.
	findingsShown = 100
	// sawShown is how much of an open finding's evidence a review is told,
	// in characters, so it can look again (0215).
	sawShown = 400
	// QuestionMax is the longest question a person may ask, in characters.
	QuestionMax = 2000
)

// Settings are a review's: the folder whose advisor it is, by ID and name
// (0212), the Anthropic API key, the model, whether it may search the web,
// and the most it may cost, in US dollars. BaseURL, where set, is the
// API's, for tests.
type Settings struct {
	Scope     string
	ScopeName string
	Key       string
	Model     string
	Web       bool
	Budget    float64
	BaseURL   string
}

// Store keeps the reviews and their findings: the conditions store.
type Store interface {
	BeginAdviceRun(scope, why, question, model string) (int64, error)
	UpdateAdviceRun(conditions.AdviceRun) error
	RecordFinding(run int64, f conditions.Finding) (int64, bool, error)
	ResolveFinding(scope, key string, run int64, why string) (int64, error)
	Findings(scope string, limit int) ([]conditions.Finding, error)
}

// Review runs one review, with the reads srv offers, and returns how it
// went. why says what asked for it: the schedule, or who. With a question,
// it answers that, and its summary is the answer (0215).
func Review(ctx context.Context, set Settings, srv *mcp.Server, store Store, why, question string, now time.Time) (conditions.AdviceRun, error) {
	if set.Model == "" {
		set.Model = "claude-opus-5-5"
	}
	question = strings.TrimSpace(question)
	id, err := store.BeginAdviceRun(set.Scope, why, question, set.Model)
	if err != nil {
		return conditions.AdviceRun{}, err
	}
	run := conditions.AdviceRun{ID: id, Scope: set.Scope, Why: why, Question: question, Model: set.Model, Outcome: "running"}
	var sp spend
	end := func(outcome, summary, fault string) (conditions.AdviceRun, error) {
		run.Outcome, run.Summary, run.Error = outcome, summary, fault
		run.Requests, run.Input, run.Output, run.CacheRead, run.CacheWrite, run.Searches, run.Cost =
			sp.requests, sp.input, sp.output, sp.read, sp.write, sp.searches, sp.cost
		return run, store.UpdateAdviceRun(run)
	}
	if set.Key == "" {
		return end("failed", "", "no Anthropic API key is set (system.advisor.key)")
	}
	r, closeReads, err := openReads(ctx, srv)
	if err != nil {
		return end("failed", "", "the manager's reads could not be opened: "+err.Error())
	}
	defer closeReads()
	so, err := store.Findings(set.Scope, findingsShown)
	if err != nil {
		return end("failed", "", "the findings so far could not be read: "+err.Error())
	}

	opts := []option.RequestOption{option.WithAPIKey(set.Key), option.WithRequestTimeout(10 * time.Minute), option.WithMaxRetries(2)}
	if set.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(set.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	tools := append(r.params(), recordFinding(), resolveFinding())
	if set.Web {
		tools = append(tools, anthropic.BetaToolUnionParam{OfWebSearchTool20260209: &anthropic.BetaWebSearchTool20260209Param{MaxUses: anthropic.Int(searches)}})
	}
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(set.Model),
		MaxTokens: maxOut,
		// The instructions and the tools are the same at every request:
		// cached together, and the conversation so far with them.
		System:       []anthropic.BetaTextBlockParam{{Text: instructions, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}},
		CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		Tools:        tools,
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort:     anthropic.BetaOutputConfigEffortHigh,
			TaskBudget: anthropic.BetaTokenTaskBudgetParam{Total: taskBudget},
		},
		// A request the model declines is answered by another, the API's
		// choice by the kind of refusal.
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01, anthropic.AnthropicBetaTaskBudgets2026_03_13},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(opening(now, so, set, question)))},
	}

	var prompt, since int64
	for {
		if sp.requests >= maxRequests {
			return end("budget", "", fmt.Sprintf("stopped after %d requests, the most a review makes", maxRequests))
		}
		maySearch := 0
		if set.Web {
			maySearch = searches
		}
		if sp.cost+next(set.Model, prompt, since, maxOut, maySearch) > set.Budget {
			return end("budget", "", fmt.Sprintf("stopped at $%.2f of its $%.2f budget: the next request could pass it", sp.cost, set.Budget))
		}
		resp, err := client.Beta.Messages.New(ctx, params)
		if err != nil {
			return end("failed", "", apiFault(err))
		}
		u := resp.Usage
		sp.add(string(resp.Model), u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens, u.ServerToolUse.WebSearchRequests)
		prompt = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens + u.OutputTokens
		run.Requests, run.Cost = sp.requests, sp.cost
		store.UpdateAdviceRun(withSpend(run, sp))
		// Append-only: the answer as it came, thinking and all.
		params.Messages = append(params.Messages, resp.ToParam())

		switch resp.StopReason {
		case anthropic.BetaStopReasonRefusal:
			return end("failed", "", "the model declined to go on ("+string(resp.StopDetails.Category)+")")
		case anthropic.BetaStopReasonPauseTurn:
			// A web search under way: the same request again goes on with it.
			since = 0
			continue
		}
		var results []anthropic.BetaContentBlockParamUnion
		var size int
		for _, block := range resp.Content {
			use, ok := block.AsAny().(anthropic.BetaToolUseBlock)
			if !ok {
				continue
			}
			text, isErr := "", false
			switch use.Name {
			case "record_finding":
				text, isErr = record(store, set.Scope, id, []byte(use.JSON.Input.Raw()))
			case "resolve_finding":
				text, isErr = resolve(store, set.Scope, id, []byte(use.JSON.Input.Raw()))
			default:
				text, isErr = r.call(ctx, use.Name, json.RawMessage(use.JSON.Input.Raw()))
			}
			size += len(text)
			results = append(results, anthropic.NewBetaToolResultBlock(use.ID, text, isErr))
		}
		if len(results) > 0 {
			params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(results...))
			since = int64(size / 3)
			continue
		}
		if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
			params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Go on from where you stopped.")))
			since = 16
			continue
		}
		return end("done", text(resp), "")
	}
}

func withSpend(run conditions.AdviceRun, sp spend) conditions.AdviceRun {
	run.Requests, run.Input, run.Output, run.CacheRead, run.CacheWrite, run.Searches, run.Cost =
		sp.requests, sp.input, sp.output, sp.read, sp.write, sp.searches, sp.cost
	return run
}

// text is an answer's text, its summary at the end of a review.
func text(m *anthropic.BetaMessage) string {
	var b strings.Builder
	for _, block := range m.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(t.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

// opening is a review's first message: what it is for, the network or a
// question (0215), when it is, its budget, and the findings so far, so it
// does not raise again what is recorded or what a person dismissed, and so
// it can look again at those open, what each concerns and saw.
func opening(now time.Time, so []conditions.Finding, set Settings, question string) string {
	var b strings.Builder
	if question != "" {
		fmt.Fprintf(&b, "Answer this question from a person who runs the network. It is %s, %s.\n\n<question>\n%s\n</question>\n\n", now.UTC().Format(time.RFC3339), now.Weekday(), question)
		b.WriteString("Look into what it asks, as far as it needs and no further: this is one question, not a review of everything. End with the answer.\n\n")
	} else {
		fmt.Fprintf(&b, "Review the network now. It is %s, %s.\n\n", now.UTC().Format(time.RFC3339), now.Weekday())
	}
	// What this advisor is for (0212): its folder and what is below it.
	// The tools answer for no more.
	if set.Scope != "" {
		fmt.Fprintf(&b, "You review %q (Locations folder %s) and what is below it, and the networks its APs use: the tools answer for these alone. What you find is for the people who run them.\n\n", set.ScopeName, set.Scope)
	}
	what := "review"
	if question != "" {
		what = "answer"
	}
	fmt.Fprintf(&b, "This %s may cost at most $%.2f.", what, set.Budget)
	if !set.Web {
		b.WriteString(" It has no web search.")
	}
	b.WriteString("\n\n")
	if len(so) == 0 {
		b.WriteString("No findings are recorded yet.")
		return b.String()
	}
	b.WriteString("Findings recorded so far, the open first, by key:\n")
	open := 0
	for _, f := range so {
		fmt.Fprintf(&b, "- %s [%s, %s, %s; found %d times, last %s]: %s\n", f.Key, f.State, f.Severity, f.Kind, f.Times, f.At.UTC().Format("2006-01-02 15:04"), f.Title)
		if f.State != "open" {
			continue
		}
		open++
		if c := concerns(f.Subjects); c != "" {
			fmt.Fprintf(&b, "  Concerns: %s\n", c)
		}
		fmt.Fprintf(&b, "  Saw: %s\n", clip(f.Saw, sawShown))
	}
	if open > 0 {
		if question == "" {
			b.WriteString("\nLook again at each open finding before you end: resolve it with resolve_finding where its problem is gone, record it again where it is still there, and leave it where you cannot tell.")
		} else {
			b.WriteString("\nWhere what you look into shows an open finding's problem gone, resolve it with resolve_finding.")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// concerns says what a finding concerns, for a review to look at again.
func concerns(s conditions.Subjects) string {
	var parts []string
	for _, p := range []struct {
		what string
		list []string
	}{{"clients", s.Clients}, {"APs", s.APs}, {"networks", s.Networks}} {
		if len(p.list) > 0 {
			parts = append(parts, p.what+" "+strings.Join(p.list, ", "))
		}
	}
	return strings.Join(parts, "; ")
}

// clip keeps the first n characters of s, on a word, and says it was cut.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	end := n
	for end > 0 && !utf8Start(s[end]) {
		end--
	}
	if i := strings.LastIndexByte(s[:end], ' '); i > n/2 {
		end = i
	}
	return s[:end] + " …"
}

var keyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

// record keeps a finding a review by scope's advisor made, and says so; or
// says what is wrong with it.
func record(store Store, scope string, run int64, input []byte) (string, bool) {
	var f finding
	if err := json.Unmarshal(input, &f); err != nil {
		return "the finding is not what record_finding takes: " + err.Error(), true
	}
	switch {
	case !keyRE.MatchString(f.Key):
		return "key: 1 to 80 lowercase letters, digits and hyphens, starting with a letter or digit", true
	case !oneOf(f.Kind, "client", "compatibility", "ap", "network", "config"):
		return "kind: client, compatibility, ap, network or config", true
	case !oneOf(f.Severity, "info", "warning", "critical"):
		return "severity: info, warning or critical", true
	case f.Title == "" || len(f.Title) > 200:
		return "title: 1 to 200 characters", true
	case f.Saw == "" || len(f.Saw) > 4000:
		return "saw: 1 to 4000 characters", true
	case f.Todo == "" || len(f.Todo) > 2000:
		return "todo: 1 to 2000 characters", true
	case len(f.Clients) > 50 || len(f.APs) > 50 || len(f.Networks) > 50:
		return "at most 50 clients, APs and networks", true
	}
	id, isNew, err := store.RecordFinding(run, conditions.Finding{
		Scope: scope, Key: f.Key, Kind: f.Kind, Severity: f.Severity, Title: f.Title, Saw: f.Saw, Todo: f.Todo,
		Subjects: conditions.Subjects{Clients: short(f.Clients), APs: short(f.APs), Networks: short(f.Networks)},
	})
	if err != nil {
		return "it could not be kept: " + err.Error(), true
	}
	if isNew {
		return fmt.Sprintf("recorded, finding %d", id), false
	}
	return fmt.Sprintf("recorded again: finding %d, already recorded under that key", id), false
}

// resolve resolves an open finding of scope's advisor whose problem review
// run found gone, and says so; or says what is wrong (0215).
func resolve(store Store, scope string, run int64, input []byte) (string, bool) {
	var r resolution
	if err := json.Unmarshal(input, &r); err != nil {
		return "this is not what resolve_finding takes: " + err.Error(), true
	}
	switch {
	case !keyRE.MatchString(r.Key):
		return "key: the key of a finding recorded before", true
	case r.Why == "" || len(r.Why) > 2000:
		return "why: 1 to 2000 characters", true
	}
	id, err := store.ResolveFinding(scope, r.Key, run, r.Why)
	if errors.Is(err, conditions.ErrNoFinding) {
		return fmt.Sprintf("no open finding has the key %q: only an open one is resolved", r.Key), true
	}
	if err != nil {
		return "it could not be resolved: " + err.Error(), true
	}
	return fmt.Sprintf("resolved, finding %d", id), false
}

func oneOf(s string, list ...string) bool {
	for _, x := range list {
		if s == x {
			return true
		}
	}
	return false
}

// short keeps the names of a finding's subjects to a sensible length.
func short(list []string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" && len(s) <= 64 {
			out = append(out, s)
		}
	}
	return out
}

// apiFault says why a request to the API failed, for the review's record.
func apiFault(err error) string {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "Anthropic's API refused the key (system.advisor.key)"
		case http.StatusTooManyRequests:
			return "Anthropic's API is rate limiting this key: the next review tries again"
		case http.StatusBadRequest:
			return "Anthropic's API refused the request: " + apierr.Error()
		}
		return fmt.Sprintf("Anthropic's API answered %d", apierr.StatusCode)
	}
	if errors.Is(err, context.Canceled) {
		return "the manager stopped during the review"
	}
	return "Anthropic's API could not be reached: " + err.Error()
}
