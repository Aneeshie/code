package agent

import (
	"strings"
	"testing"
)

func TestSystemPromptStatesKeyRules(t *testing.T) {
	for _, want := range []string{
		"NO shell",
		"Never claim to have run",
		"Not run / not verified",
		"Do NOT fall back to write_file",
		"read_file",
		"call git_diff",
	} {
		if !strings.Contains(SystemPrompt, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}
