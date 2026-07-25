package imports

import (
	"context"
	"testing"
)

type fakeSpec struct {
	name      string
	rejection *ImportRejection
}

func (s fakeSpec) Name() string { return s.name }
func (s fakeSpec) IsSatisfiedBy(context.Context, *ImportCandidate) *ImportRejection {
	return s.rejection
}

func TestDecisionMakerEvaluateCollectsRejections(t *testing.T) {
	t.Parallel()
	dm := NewDecisionMaker(
		fakeSpec{name: "ok-1"},
		fakeSpec{name: "reject-1", rejection: &ImportRejection{
			Reason:  RejectionSample,
			Message: "sample file",
		}},
		fakeSpec{name: "reject-2", rejection: &ImportRejection{
			Reason:  RejectionDangerousFile,
			Message: "dangerous extension",
		}},
	)

	eval := dm.Evaluate(context.Background(), &ImportCandidate{SourcePath: "/downloads/file.mkv"})
	if eval.Approved() {
		t.Fatal("expected evaluation to be rejected")
	}
	if len(eval.Rejections) != 2 {
		t.Fatalf("expected 2 rejections, got %d", len(eval.Rejections))
	}
	if eval.Rejections[0].Reason != RejectionSample || eval.Rejections[1].Reason != RejectionDangerousFile {
		t.Fatalf("unexpected rejections: %+v", eval.Rejections)
	}
}

func TestDecisionMakerEvaluateApproved(t *testing.T) {
	t.Parallel()
	dm := NewDecisionMaker(fakeSpec{name: "ok-1"}, fakeSpec{name: "ok-2"})
	eval := dm.Evaluate(context.Background(), &ImportCandidate{SourcePath: "/downloads/file.mkv"})
	if !eval.Approved() {
		t.Fatalf("expected approval, got rejections: %+v", eval.Rejections)
	}
	if len(eval.Rejections) != 0 {
		t.Fatalf("expected 0 rejections, got %d", len(eval.Rejections))
	}
}
