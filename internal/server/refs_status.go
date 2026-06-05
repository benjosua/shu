package server

type EntityRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func ref(typ, id string) EntityRef {
	return EntityRef{Type: typ, ID: id}
}

func (r EntityRef) valid() bool {
	return r.Type != "" && r.ID != ""
}

const (
	RoleTypeUser      = "user"
	RoleTypeAgent     = "agent"
	RoleTypeSquad     = "squad"
	EntityIssue       = "issue"
	EntityComment     = "comment"
	EntityAttachment  = "attachment"
	EntityWork        = "work"
	EntityArtifact    = "artifact"
	EntityItem        = "item"
	EntityResource    = "resource"
	EntityAction      = "external_action"
	EntitySyncRun     = "external_sync_run"
	EntityAutopilot   = "autopilot"
	EntityChatSession = "chat_session"
	EntityChatMessage = "chat_message"
	EntityLabel       = "label"
)

const (
	ExecutorOnline  = "online"
	ExecutorOffline = "offline"
)

const (
	WorkQueued     = "queued"
	WorkDispatched = "dispatched"
	WorkRunning    = "running"
	WorkCompleted  = "completed"
	WorkFailed     = "failed"
	WorkCancelled  = "cancelled"
)

const (
	ActionPending   = "pending"
	ActionApproved  = "approved"
	ActionRunning   = "running"
	ActionSucceeded = "succeeded"
	ActionFailed    = "failed"
	ActionCancelled = "cancelled"
)

const (
	IssueBacklog = "backlog"
	IssueTodo    = "todo"
	IssueDone    = "done"
)

const (
	TodoOpen      = "open"
	TodoCompleted = "completed"
)
