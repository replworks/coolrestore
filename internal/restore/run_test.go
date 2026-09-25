package restore

import (
	"reflect"
	"testing"
)

func TestRunDefaultsToPlanOnlyAndDoesNotApply(t *testing.T) {
	planned := false
	applied := false

	plan, outcome, err := Run(false,
		func() (Plan, error) {
			planned = true
			return Plan{}, nil
		},
		func(Plan) error {
			applied = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if plan.Mode != "" {
		t.Fatalf("unauthorized plan = %#v, want empty test plan", plan)
	}
	if outcome != OutcomePlanOnly {
		t.Fatalf("Run() outcome = %v, want plan-only", outcome)
	}
	if !planned {
		t.Fatal("planner was not called")
	}
	if applied {
		t.Fatal("change application was called for an unauthorized invocation")
	}
}

func TestRunAppliesOnlyAfterAuthorizedPlanning(t *testing.T) {
	planned := false
	applied := false

	plan, outcome, err := Run(true,
		func() (Plan, error) {
			planned = true
			return Plan{}, nil
		},
		func(Plan) error {
			if !planned {
				t.Fatal("change application ran before planning")
			}
			applied = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if plan.Mode != "" {
		t.Fatalf("authorized plan = %#v, want empty test plan", plan)
	}
	if outcome != OutcomeApplied || !applied {
		t.Fatalf("Run() outcome = %v, applied = %v", outcome, applied)
	}
}

func TestRunPassesTheComputedPlanToApplicationWithoutReplanning(t *testing.T) {
	expected := Plan{
		Mode:         ModeMerge,
		Added:        []string{"new.txt"},
		Overwritten:  []string{"existing.txt"},
		RegularFiles: 2,
	}
	plannerCalls := 0
	var applied Plan

	preview, previewOutcome, err := Run(false,
		func() (Plan, error) {
			plannerCalls++
			return expected, nil
		},
		func(Plan) error {
			t.Fatal("preview unexpectedly called change application")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("preview Run() error = %v", err)
	}
	if previewOutcome != OutcomePlanOnly {
		t.Fatalf("preview outcome = %v, want plan-only", previewOutcome)
	}

	execution, executionOutcome, err := Run(true,
		func() (Plan, error) {
			plannerCalls++
			return expected, nil
		},
		func(plan Plan) error {
			applied = plan
			return nil
		},
	)
	if err != nil {
		t.Fatalf("execution Run() error = %v", err)
	}
	if executionOutcome != OutcomeApplied {
		t.Fatalf("execution outcome = %v, want applied", executionOutcome)
	}
	if plannerCalls != 2 {
		t.Fatalf("planner calls = %d, want one per independent invocation", plannerCalls)
	}
	if !reflect.DeepEqual(preview, execution) {
		t.Fatalf("preview plan = %#v, execution plan = %#v", preview, execution)
	}
	if !reflect.DeepEqual(execution, applied) {
		t.Fatalf("application received %#v, execution returned %#v", applied, execution)
	}
}

func TestRunComputesAPlanOnceBeforeApplyingIt(t *testing.T) {
	expected := Plan{Mode: ModeReplace, ResultPaths: []string{"file.txt"}}
	plannerCalls := 0

	_, outcome, err := Run(true,
		func() (Plan, error) {
			plannerCalls++
			return expected, nil
		},
		func(plan Plan) error {
			if !reflect.DeepEqual(plan, expected) {
				t.Fatalf("application received %#v, want %#v", plan, expected)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome != OutcomeApplied {
		t.Fatalf("Run() outcome = %v, want applied", outcome)
	}
	if plannerCalls != 1 {
		t.Fatalf("planner calls = %d, want 1", plannerCalls)
	}
}
