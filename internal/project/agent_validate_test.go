package project

import (
	"strings"
	"testing"
)

func TestValidate_AgentTypes(t *testing.T) {
	for _, a := range []string{"claude", "codex", "amp", "opencode", "drovercode", "drover-code", ""} {
		c := DefaultConfig()
		c.Agent = a
		if err := c.Validate(); err != nil {
			t.Errorf("agent %q: unexpected error %v", a, err)
		}
	}
	c := DefaultConfig()
	c.Agent = "copilot"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown agent type: copilot") || !strings.Contains(err.Error(), "drovercode") {
		t.Errorf("want unknown-agent error listing drovercode, got %v", err)
	}
}
