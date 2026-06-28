package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/wonderjl/no-mistakes/internal/types"
)

func TestExecutor_ApprovalFix(t *testing.T) {
	database, p, run, repo := setupTest(t)
	workDir := t.TempDir()

	// Step that needs approval on first call, passes on second
	callCount := 0
	var step Step = &adaptiveCallStep{
		name: types.StepReview,
		fn: func(sctx *StepContext) (*StepOutcome, error) {
			callCount++
			if callCount == 1 {
				return &StepOutcome{NeedsApproval: true, Findings: `{"issues":["bug"]}`}, nil
			}
			// After fix, re-evaluate passes
			return &StepOutcome{NeedsApproval: false, ExitCode: 0}, nil
		},
	}

	steps := []Step{step, newPassStep(types.StepTest)}
	exec := NewExecutor(database, p, nil, nil, steps, nil)

	done := make(chan error, 1)
	go func() {
		done <- exec.Execute(context.Background(), run, repo, workDir)
	}()

	// Wait for awaiting_approval
	waitForStepStatus(t, database, run.ID, types.StepReview, types.StepStatusAwaitingApproval)

	// Send fix action
	exec.Respond(types.StepReview, types.ActionFix, nil)

	// Wait for step to re-execute and complete (it passes on second call)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("executor timed out")
	}

	// Both steps should be completed
	dbSteps, _ := database.GetStepsByRun(run.ID)
	if dbSteps[0].Status != types.StepStatusCompleted {
		t.Errorf("review: expected %q, got %q", types.StepStatusCompleted, dbSteps[0].Status)
	}
	if dbSteps[1].Status != types.StepStatusCompleted {
		t.Errorf("test: expected %q, got %q", types.StepStatusCompleted, dbSteps[1].Status)
	}

	// Step should have been called twice (initial + after fix)
	if callCount != 2 {
		t.Errorf("expected step to be called 2 times, got %d", callCount)
	}
}

// Telemetry assertions for approval/fix were removed with the telemetry
// subsystem. The underlying approval-and-fix flow is covered by
// TestExecutor_ApprovalFix above, and auto-fix by executor_autofix_test.go.
