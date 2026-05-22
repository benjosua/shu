package daemon

import "encoding/json"

type ClaimedWork struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id"`
	Title            string          `json:"title"`
	Body             string          `json:"body"`
	AgentID          string          `json:"agent_id"`
	ExecutorID       string          `json:"executor_id"`
	ExecutorMode     string          `json:"executor_mode"`
	PriorSessionID   string          `json:"prior_session_id"`
	PriorWorkDir     string          `json:"prior_work_dir"`
	StepInstructions string          `json:"step_instructions"`
	Resource         WorkResource    `json:"resource"`
	Agent            ClaimedAgent    `json:"agent"`
	raw              json.RawMessage `json:"-"`
}

type WorkResource struct {
	Kind    string `json:"kind"`
	Locator string `json:"locator"`
}

type ClaimedAgent struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	Instructions string            `json:"instructions"`
	Model        string            `json:"model"`
	CustomEnv    map[string]string `json:"custom_env"`
	CustomArgs   []string          `json:"custom_args"`
}

type WorkEnv struct {
	RootDir string
	WorkDir string
	Env     map[string]string
}
