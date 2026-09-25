package restore

import "testing"

func TestRunDefaultsToPlanOnlyAndDoesNotApply(t *testing.T) {
	planned := false
	applied := false

	outcome, err := Run(false,
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

	outcome, err := Run(true,
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
	if outcome != OutcomeApplied || !applied {
		t.Fatalf("Run() outcome = %v, applied = %v", outcome, applied)
	}
}
