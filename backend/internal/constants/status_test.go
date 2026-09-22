package constants

import "testing"

func TestSamplingBatchTransitionGraph(t *testing.T) {
	if !CanTransition(SamplingBatchTransitions, "planned", "collecting") {
		t.Fatalf("expected planned -> collecting transition to be allowed")
	}
	if CanTransition(SamplingBatchTransitions, "planned", "unknown") {
		t.Fatal("unknown status must never be accepted")
	}
}

func TestResultReviewRequiresPeerReviewBeforeSigning(t *testing.T) {
	if CanTransition(ResultReviewTransitions, "draft", "signed") {
		t.Fatal("draft review must not be signed without peer review")
	}
	if !CanTransition(ResultReviewTransitions, "draft", "peer_review") ||
		!CanTransition(ResultReviewTransitions, "peer_review", "signed") {
		t.Fatal("expected draft -> peer_review -> signed workflow")
	}
}

func TestReleaseGateStatusHelpers(t *testing.T) {
	if !AssayMethodUsable("active") || AssayMethodUsable("validated") || AssayMethodUsable("retired") {
		t.Fatal("only active method versions may back reception and signing")
	}
	if !SamplingBatchReceived("received") || SamplingBatchReceived("collecting") || SamplingBatchReceived("closed") {
		t.Fatal("only received batches may accept new samples")
	}
}
