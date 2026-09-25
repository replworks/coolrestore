// Package restore contains the boundary between restore planning and change
// application.
package restore

// Plan is the single value that will flow from Restore Planner to Change
// Application. Its contents are introduced by the planning task; the
// authorization gate does not inspect or recompute it.
type Plan struct{}

// Outcome identifies how a valid invocation ended at this stage.
type Outcome uint8

const (
	OutcomePlanOnly Outcome = iota
	OutcomeApplied
)

// Run always completes planning first. Authorization is checked only after
// planning, and an unauthorized invocation never calls apply. The callbacks
// deliberately do not receive authorization as a planner input.
func Run(authorized bool, plan func() (Plan, error), apply func(Plan) error) (Outcome, error) {
	computedPlan, err := plan()
	if err != nil {
		return OutcomePlanOnly, err
	}
	if !authorized {
		return OutcomePlanOnly, nil
	}
	if err := apply(computedPlan); err != nil {
		return OutcomeApplied, err
	}
	return OutcomeApplied, nil
}
