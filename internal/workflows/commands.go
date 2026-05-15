package workflows

// Command is the sealed interface for all orchestrator commands.
// Every workflow state mutation flows through the orchestrator's command channel.
type Command interface {
	commandTag()
}

// SearchReply is the response for synchronous CmdSearchStarted.
type SearchReply struct {
	Workflow *Workflow
	Err      error
}

// CmdSearchStarted creates a new workflow. Synchronous: callers need the
// workflow ID back to later send CmdGrabbed.
type CmdSearchStarted struct {
	WfType           string
	MediaType        string
	QualityProfileID string
	MediaIDs         []string
	Reply            chan SearchReply
}

func (CmdSearchStarted) commandTag() {}

// CmdGrabbed records that a release was grabbed (searching → grabbed).
type CmdGrabbed struct {
	WorkflowID           string
	ClientID             string
	DownloadID           string
	Title                string
	SeedRatioLimit       *float64
	SeedTimeLimitMinutes *int
}

func (CmdGrabbed) commandTag() {}

// CmdDownloadProgress is a low-priority update coalesced per tick.
// Does not trigger state transitions — only updates metadata.
type CmdDownloadProgress struct {
	ClientID   string
	DownloadID string
	Progress   float64
	DownSpeed  int64
	UpSpeed    int64
	Ratio      float64
	Status     string // download client item status (e.g. "seeding", "completed")
}

func (CmdDownloadProgress) commandTag() {}

// CmdDownloadComplete signals that a download has finished (detected by monitor).
// The orchestrator resolves the workflow by clientID+downloadID.
type CmdDownloadComplete struct {
	ClientID   string
	DownloadID string
	Title      string
	Category   string
}

func (CmdDownloadComplete) commandTag() {}

// CmdImportResult is sent by import workers back to the orchestrator.
type CmdImportResult struct {
	WorkflowID    string
	Success       bool
	Error         string
	ImportedPaths []string
}

func (CmdImportResult) commandTag() {}

// CmdCancel cancels an active workflow. Synchronous for immediate feedback.
type CmdCancel struct {
	WorkflowID string
	Reply      chan error
}

func (CmdCancel) commandTag() {}

// CmdRetry restarts a failed workflow. Synchronous for immediate feedback.
type CmdRetry struct {
	WorkflowID string
	Reply      chan error
}

func (CmdRetry) commandTag() {}

// CmdTick is an internal command for periodic health checks.
// Sent by the orchestrator's own ticker, not by external callers.
type CmdTick struct{}

func (CmdTick) commandTag() {}
