package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/tools"
)

// scriptedClient replays a fixed sequence of assistant messages and records
// the history it was called with.
type scriptedClient struct {
	steps     []client.Message
	calls     int
	histories [][]client.Message
	defs      []client.ToolDefinition
	err       error
}

func (c *scriptedClient) Send(context.Context, []client.Message) (*client.Response, error) {
	return nil, errors.New("not implemented")
}

func (c *scriptedClient) Stream(_ context.Context, messages []client.Message, defs []client.ToolDefinition, _ func(string)) (*client.StreamEvent, error) {
	c.histories = append(c.histories, append([]client.Message(nil), messages...))
	c.defs = defs
	c.calls++

	if c.err != nil {
		return nil, c.err
	}
	if c.calls > len(c.steps) {
		return nil, fmt.Errorf("scripted client exhausted after %d steps", len(c.steps))
	}
	return &client.StreamEvent{Message: c.steps[c.calls-1]}, nil
}

// fakeTool is a configurable tool that counts invocations.
type fakeTool struct {
	name   string
	output string
	err    error
	calls  []string
}

func (f *fakeTool) Name() string                { return f.name }
func (f *fakeTool) Description() string         { return "fake " + f.name }
func (f *fakeTool) InputSchema() map[string]any { return map[string]any{"type": "object"} }
func (f *fakeTool) Execute(_ context.Context, input string) (string, error) {
	f.calls = append(f.calls, input)
	return f.output, f.err
}

func assistantCall(id, name, args string) client.Message {
	return client.Message{
		Role:      client.RoleAssistant,
		ToolCalls: []client.ToolCall{{ID: id, Name: name, Arguments: args}},
	}
}

func newTestAgent(c client.LLMClient, ts ...tools.Tool) *Agent {
	reg := tools.NewRegistry()
	for _, tool := range ts {
		reg.Register(tool)
	}
	return NewAgent(c, reg)
}

func userHistory() []client.Message {
	return []client.Message{{Role: client.RoleUser, Content: "go"}}
}

func TestAgentFinishesWithoutToolCalls(t *testing.T) {
	c := &scriptedClient{steps: []client.Message{{Role: client.RoleAssistant, Content: "done"}}}
	a := newTestAgent(c)

	history, err := a.Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.calls != 1 {
		t.Fatalf("client called %d times, want 1", c.calls)
	}
	if len(history) != 2 || history[1].Content != "done" {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestAgentExecutesToolCallsAndLoopsUntilDone(t *testing.T) {
	tool := &fakeTool{name: "echo", output: "tool-output"}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "echo", `{"n":1}`),
		assistantCall("2", "echo", `{"n":2}`),
		{Role: client.RoleAssistant, Content: "all done"},
	}}

	history, err := newTestAgent(c, tool).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.calls != 3 {
		t.Fatalf("client called %d times, want 3", c.calls)
	}
	if len(tool.calls) != 2 || tool.calls[0] != `{"n":1}` || tool.calls[1] != `{"n":2}` {
		t.Fatalf("tool calls = %v", tool.calls)
	}

	// user, (assistant, tool) x2, final assistant
	if len(history) != 6 {
		t.Fatalf("history length = %d, want 6: %+v", len(history), history)
	}
	for _, i := range []int{2, 4} {
		m := history[i]
		if m.Role != client.RoleTool || m.Content != "tool-output" || m.ToolCallID != fmt.Sprint(i/2) {
			t.Errorf("history[%d] = %+v", i, m)
		}
	}

	// Each model call must see the tool results from the previous round.
	if got := len(c.histories[2]); got != 5 {
		t.Fatalf("third call saw %d messages, want 5", got)
	}
}

func TestAgentHandlesMultipleToolCallsInOneTurn(t *testing.T) {
	tool := &fakeTool{name: "echo", output: "ok"}
	c := &scriptedClient{steps: []client.Message{
		{Role: client.RoleAssistant, ToolCalls: []client.ToolCall{
			{ID: "a", Name: "echo", Arguments: `{}`},
			{ID: "b", Name: "echo", Arguments: `{}`},
		}},
		{Role: client.RoleAssistant, Content: "done"},
	}}

	history, err := newTestAgent(c, tool).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tool.calls) != 2 {
		t.Fatalf("tool executed %d times, want 2", len(tool.calls))
	}
	if history[2].ToolCallID != "a" || history[3].ToolCallID != "b" {
		t.Fatalf("tool results out of order: %+v", history[2:4])
	}
}

func TestAgentReportsToolErrorsToModelAndContinues(t *testing.T) {
	tool := &fakeTool{name: "flaky", err: errors.New("boom")}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "flaky", `{}`),
		{Role: client.RoleAssistant, Content: "recovered"},
	}}

	history, err := newTestAgent(c, tool).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("tool errors must not abort the run: %v", err)
	}

	result := history[2]
	if result.Role != client.RoleTool || !strings.Contains(result.Content, "Tool call failed: boom") {
		t.Fatalf("tool error not forwarded to model: %+v", result)
	}
}

func TestAgentReportsUnknownToolToModel(t *testing.T) {
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "does_not_exist", `{}`),
		{Role: client.RoleAssistant, Content: "ok"},
	}}

	history, err := newTestAgent(c).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(history[2].Content, "not found") {
		t.Fatalf("unknown tool not reported: %+v", history[2])
	}
}

func TestAgentPassesToolDefinitionsToClient(t *testing.T) {
	c := &scriptedClient{steps: []client.Message{{Role: client.RoleAssistant, Content: "hi"}}}
	a := newTestAgent(c, &fakeTool{name: "one"}, &fakeTool{name: "two"})

	if _, err := a.Run(context.Background(), userHistory(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := map[string]bool{}
	for _, d := range c.defs {
		names[d.Name] = true
		if d.InputSchema == nil {
			t.Errorf("definition %q has no schema", d.Name)
		}
	}
	if len(c.defs) != 2 || !names["one"] || !names["two"] {
		t.Fatalf("definitions = %+v", c.defs)
	}
}

func TestAgentPropagatesClientError(t *testing.T) {
	want := errors.New("network down")
	c := &scriptedClient{err: want}

	history, err := newTestAgent(c).Run(context.Background(), userHistory(), nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if history != nil {
		t.Fatalf("history should be nil on error, got %+v", history)
	}
}

// loopingClient always asks for another tool call, simulating a model stuck
// in a tool-call loop.
type loopingClient struct{ calls int }

func (c *loopingClient) Send(context.Context, []client.Message) (*client.Response, error) {
	return nil, errors.New("not implemented")
}

func (c *loopingClient) Stream(context.Context, []client.Message, []client.ToolDefinition, func(string)) (*client.StreamEvent, error) {
	c.calls++
	return &client.StreamEvent{Message: assistantCall(fmt.Sprint(c.calls), "echo", `{}`)}, nil
}

func TestAgentStopsRunawayToolCallLoop(t *testing.T) {
	tool := &fakeTool{name: "echo", output: "again"}
	c := &loopingClient{}
	a := newTestAgent(c, tool)
	a.MaxIterations = 5

	_, err := a.Run(context.Background(), userHistory(), nil)
	if !errors.Is(err, ErrMaxIterations) {
		t.Fatalf("error = %v, want ErrMaxIterations", err)
	}
	if c.calls != 5 {
		t.Fatalf("client called %d times, want exactly 5", c.calls)
	}
	if len(tool.calls) != 5 {
		t.Fatalf("tool executed %d times, want 5", len(tool.calls))
	}
}

func TestAgentHasDefaultIterationLimit(t *testing.T) {
	a := newTestAgent(&loopingClient{})
	if a.MaxIterations <= 0 {
		t.Fatalf("default MaxIterations = %d, want > 0", a.MaxIterations)
	}
}

func TestAgentStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tool := &cancellingTool{cancel: cancel}
	c := &loopingClient{}
	a := newTestAgent(c, tool)

	_, err := a.Run(ctx, userHistory(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if c.calls != 1 {
		t.Fatalf("client called %d times after cancel, want 1", c.calls)
	}
}

type cancellingTool struct{ cancel context.CancelFunc }

func (c *cancellingTool) Name() string                { return "echo" }
func (c *cancellingTool) Description() string         { return "cancels the context" }
func (c *cancellingTool) InputSchema() map[string]any { return map[string]any{"type": "object"} }
func (c *cancellingTool) Execute(context.Context, string) (string, error) {
	c.cancel()
	return "ok", nil
}

// mutatingTool is a fakeTool that reports itself as changing the workspace.
type mutatingTool struct{ fakeTool }

func (m *mutatingTool) Mutates() bool { return true }

func final(text string) client.Message {
	return client.Message{Role: client.RoleAssistant, Content: text}
}

func countReminders(history []client.Message) int {
	n := 0
	for _, m := range history {
		if m.Role == client.RoleUser && m.Content == verificationReminder {
			n++
		}
	}
	return n
}

func TestAgentRequiresVerificationAfterChanges(t *testing.T) {
	edit := &mutatingTool{fakeTool{name: "edit_file", output: "modified a.go"}}
	diff := &fakeTool{name: VerificationTool, output: "M a.go"}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "edit_file", `{}`),
		final("all done"), // tries to finish without verifying
		assistantCall("2", VerificationTool, `{}`),
		final("verified, done"),
	}}

	history, err := newTestAgent(c, edit, diff).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diff.calls) != 1 {
		t.Fatalf("git_diff called %d times, want 1", len(diff.calls))
	}
	if got := countReminders(history); got != 1 {
		t.Fatalf("reminders = %d, want 1", got)
	}
	if last := history[len(history)-1]; last.Content != "verified, done" {
		t.Fatalf("final message = %q", last.Content)
	}
	if c.calls != 4 {
		t.Fatalf("client called %d times, want 4", c.calls)
	}
}

func TestAgentDoesNotNudgeWhenAlreadyVerified(t *testing.T) {
	edit := &mutatingTool{fakeTool{name: "edit_file", output: "ok"}}
	diff := &fakeTool{name: VerificationTool, output: "M a.go"}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "edit_file", `{}`),
		assistantCall("2", VerificationTool, `{}`),
		final("done"),
	}}

	history, err := newTestAgent(c, edit, diff).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countReminders(history); got != 0 {
		t.Fatalf("reminders = %d, want 0", got)
	}
}

func TestAgentRequiresReverificationAfterLaterChange(t *testing.T) {
	edit := &mutatingTool{fakeTool{name: "edit_file", output: "ok"}}
	diff := &fakeTool{name: VerificationTool, output: "M a.go"}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "edit_file", `{}`),
		assistantCall("2", VerificationTool, `{}`),
		assistantCall("3", "edit_file", `{}`), // changes again after verifying
		final("done"),                         // verification is now stale
		assistantCall("4", VerificationTool, `{}`),
		final("really done"),
	}}

	history, err := newTestAgent(c, edit, diff).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countReminders(history); got != 1 {
		t.Fatalf("reminders = %d, want 1", got)
	}
	if len(diff.calls) != 2 {
		t.Fatalf("git_diff called %d times, want 2", len(diff.calls))
	}
}

func TestAgentDoesNotNudgeWithoutChanges(t *testing.T) {
	reader := &fakeTool{name: "read_file", output: "contents"}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "read_file", `{}`),
		final("here is the answer"),
	}}

	history, err := newTestAgent(c, reader).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countReminders(history); got != 0 {
		t.Fatalf("reminders = %d, want 0", got)
	}
}

func TestAgentDoesNotNudgeAfterFailedChange(t *testing.T) {
	edit := &mutatingTool{fakeTool{name: "edit_file", err: errors.New("hunk failed")}}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "edit_file", `{}`),
		final("could not apply the patch"),
	}}

	history, err := newTestAgent(c, edit).Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countReminders(history); got != 0 {
		t.Fatalf("a failed change changed nothing, reminders = %d, want 0", got)
	}
}

func TestAgentGivesUpAfterMaxVerificationNudges(t *testing.T) {
	edit := &mutatingTool{fakeTool{name: "edit_file", output: "ok"}}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "edit_file", `{}`),
		final("done"),
		final("done again"),
		final("done yet again"),
	}}

	a := newTestAgent(c, edit)
	a.MaxVerificationNudges = 2

	history, err := a.Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countReminders(history); got != 2 {
		t.Fatalf("reminders = %d, want exactly 2", got)
	}
	if last := history[len(history)-1]; last.Content != "done yet again" {
		t.Fatalf("final message = %q", last.Content)
	}
}

func TestAgentVerificationCanBeDisabled(t *testing.T) {
	edit := &mutatingTool{fakeTool{name: "edit_file", output: "ok"}}
	c := &scriptedClient{steps: []client.Message{
		assistantCall("1", "edit_file", `{}`),
		final("done"),
	}}

	a := newTestAgent(c, edit)
	a.MaxVerificationNudges = 0

	history, err := a.Run(context.Background(), userHistory(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countReminders(history); got != 0 {
		t.Fatalf("reminders = %d, want 0", got)
	}
}
