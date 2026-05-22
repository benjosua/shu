package daemon

import (
	"strings"
	"testing"
)

func TestRepoResourceKinds(t *testing.T) {
	for _, kind := range []string{"git", "git_repo", "repo", "repo_url", "github_repo"} {
		if !isRepoResource(kind) {
			t.Fatalf("%s should be repo resource", kind)
		}
	}
	if isRepoResource("local_path") {
		t.Fatal("local_path should not be repo resource")
	}
}

func TestBuildWorkPromptIncludesStepAndResource(t *testing.T) {
	prompt := buildWorkPrompt(ClaimedWork{
		Title:            "Fix bug",
		Body:             "Patch it",
		StepInstructions: "Review before final",
		Resource:         WorkResource{Kind: "github_repo", Locator: "https://example/repo.git"},
		Agent:            ClaimedAgent{Name: "dev"},
	})
	for _, want := range []string{"Fix bug", "Patch it", "Review before final", "github_repo", "dev"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
