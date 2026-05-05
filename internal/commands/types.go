// Package commands implements a persistent command queue inspired by
// Radarr/Sonarr's POST /api/v1/command surface.
package commands

import "time"

// CommandName identifies a queued operation.
type CommandName string

const (
	CmdRescanMovie   CommandName = "RescanMovie"
	CmdRescanSeries  CommandName = "RescanSeries"
	CmdRefreshMovie  CommandName = "RefreshMovie"
	CmdRefreshSeries CommandName = "RefreshSeries"
	CmdMissingSearch CommandName = "MissingSearch"
	CmdCutoffSearch  CommandName = "CutoffSearch"
	CmdBackup        CommandName = "Backup"
	CmdRssSync       CommandName = "RssSync"
)

// CommandStatus tracks lifecycle state.
type CommandStatus string

const (
	StatusQueued    CommandStatus = "queued"
	StatusRunning   CommandStatus = "running"
	StatusCompleted CommandStatus = "completed"
	StatusFailed    CommandStatus = "failed"
)

// Command is a queued automation command.
type Command struct {
	ID          string        `json:"id"`
	Name        CommandName   `json:"name"`
	Body        string        `json:"body"`
	Status      CommandStatus `json:"status"`
	Priority    int           `json:"priority"`
	StartedAt   *time.Time    `json:"started_at,omitempty"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Result      *string       `json:"result,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}
