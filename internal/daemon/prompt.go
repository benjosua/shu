package daemon

import (
	"fmt"
	"strings"
)

func buildWorkPrompt(work ClaimedWork) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\n\n", work.Title)
	if work.Agent.Name != "" {
		fmt.Fprintf(&b, "You are agent %q.\n", work.Agent.Name)
	}
	if strings.TrimSpace(work.Agent.Instructions) != "" {
		fmt.Fprintf(&b, "\nAgent instructions:\n%s\n", work.Agent.Instructions)
	}
	if work.PriorSessionID != "" {
		fmt.Fprintf(&b, "Prior session id: %s\n", work.PriorSessionID)
	}
	if work.Resource.Kind != "" {
		fmt.Fprintf(&b, "Resource: %s %s\n", work.Resource.Kind, work.Resource.Locator)
	}
	if strings.TrimSpace(work.StepInstructions) != "" {
		fmt.Fprintf(&b, "\nStep instructions:\n%s\n", work.StepInstructions)
	}
	b.WriteString("\n")
	b.WriteString(work.Body)
	b.WriteString("\n")
	return b.String()
}
