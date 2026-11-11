package workflows

import (
	"context"
	"fmt"
)

// CmdDownloadReacquired re-arms a failed workflow after a user-authorized
// acquisition has successfully returned the same client/download identity.
// Ordinary monitor completions must not send this command: doing so would
// continuously retry terminal import failures without user authorization.
type CmdDownloadReacquired struct {
	ClientID   string
	DownloadID string
	Reply      chan error
}

func (CmdDownloadReacquired) commandTag() {}

// ResumeReacquiredDownload must be called only after a successful, explicitly
// authorized re-acquisition. The next monitor completion then follows normal
// settling/import dispatch, even if completion history already knows this ID.
// Active workflows and identities without a failed workflow are unchanged.
func (o *Orchestrator) ResumeReacquiredDownload(ctx context.Context, clientID, downloadID string) error {
	if clientID == "" || downloadID == "" {
		return fmt.Errorf("client and download identity are required")
	}
	reply := make(chan error, 1)
	select {
	case o.commands <- CmdDownloadReacquired{ClientID: clientID, DownloadID: downloadID, Reply: reply}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *Orchestrator) handleDownloadReacquired(ctx context.Context, cmd CmdDownloadReacquired) {
	err := o.resumeReacquiredDownload(ctx, cmd.ClientID, cmd.DownloadID)
	if cmd.Reply != nil {
		cmd.Reply <- err
	}
}

func (o *Orchestrator) resumeReacquiredDownload(ctx context.Context, clientID, downloadID string) error {
	active, err := o.store.FindByDownload(ctx, clientID, downloadID)
	if err != nil || active != nil {
		return err
	}
	failed, err := o.store.FindFailedByDownload(ctx, clientID, downloadID)
	if err != nil || failed == nil {
		return err
	}
	if !o.mediaFreeFor(ctx, failed) {
		return fmt.Errorf("another workflow owns media from the failed download")
	}
	if err := o.engine.RecoverToDownloading(ctx, failed.ID, "User-authorized download re-acquired"); err != nil {
		return err
	}
	o.logEvent(ctx, failed.ID, EventRetried, "User-authorized re-acquisition — waiting for download completion", nil)
	return nil
}
