package imports

import (
	"context"

	"github.com/ebenderooock/loom/internal/parser"
)

// ImportRejectionReason categorizes why a file was rejected.
type ImportRejectionReason string

const (
	RejectionSample            ImportRejectionReason = "sample"
	RejectionInsufficientSpace ImportRejectionReason = "insufficient_space"
	RejectionAlreadyImported   ImportRejectionReason = "already_imported"
	RejectionUnpacking         ImportRejectionReason = "unpacking"
	RejectionNotUpgrade        ImportRejectionReason = "not_upgrade"
	RejectionNoAudio           ImportRejectionReason = "no_audio"
	RejectionDangerousFile     ImportRejectionReason = "dangerous_file"
	RejectionCustomFormat      ImportRejectionReason = "custom_format_score"
)

// ImportRejection represents a single reason a file was rejected.
type ImportRejection struct {
	Reason  ImportRejectionReason
	Message string
}

// ImportCandidate holds all context needed for import decisions.
type ImportCandidate struct {
	SourcePath       string
	DestPath         string
	FileSize         int64
	Match            *MatchResult
	ImportMode       ImportMode
	IsManual         bool
	QualityProfileID string          // the media item's assigned quality profile
	ExistingQuality  string          // quality-definition name of existing file (empty if none)
	IncomingRelease  *parser.Release // parsed release info for the incoming file
	ResolvedTier     string          // quality identity to store, decided by UpgradeSpec
	// GrabbedQuality is the quality search decided at grab time. When set,
	// import reuses it instead of detecting the quality again.
	GrabbedQuality        string
	GrabbedQualityAssumed bool
	// GrabApproved means Loom grabbed this download after search checked it
	// against the profile, or the user chose it manually. Quality specs then
	// record QualityWarnings instead of rejecting the file.
	GrabApproved    bool
	GrabbedManually bool
	QualityWarnings []string
}

// ImportEvaluation holds the result of running all specs against a candidate.
type ImportEvaluation struct {
	Candidate  *ImportCandidate
	Rejections []ImportRejection
}

// Approved returns true when no spec rejected the candidate.
func (e *ImportEvaluation) Approved() bool {
	return len(e.Rejections) == 0
}

// ImportSpec is the interface all import specifications must implement.
type ImportSpec interface {
	Name() string
	IsSatisfiedBy(ctx context.Context, candidate *ImportCandidate) *ImportRejection
}

// DecisionMaker evaluates a candidate against all registered specs.
type DecisionMaker struct {
	specs []ImportSpec
}

// NewDecisionMaker creates a DecisionMaker with the given specs.
func NewDecisionMaker(specs ...ImportSpec) *DecisionMaker {
	return &DecisionMaker{specs: specs}
}

// Evaluate runs all specs and collects rejections (no short-circuit).
func (dm *DecisionMaker) Evaluate(ctx context.Context, candidate *ImportCandidate) *ImportEvaluation {
	var rejections []ImportRejection
	for _, spec := range dm.specs {
		if r := spec.IsSatisfiedBy(ctx, candidate); r != nil {
			rejections = append(rejections, *r)
		}
	}
	return &ImportEvaluation{
		Candidate:  candidate,
		Rejections: rejections,
	}
}
