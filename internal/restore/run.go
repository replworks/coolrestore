// Package restore contains the boundary between restore planning and change
// application.
package restore

// Mode identifies the target mutation strategy represented by a plan.
type Mode string

const (
	ModeMerge   Mode = "merge"
	ModeReplace Mode = "replace"
)

// Plan is the single value that flows from Restore Planner to Change
// Application. Paths are relative to the target and use slash separators so
// the plan is stable across operating systems.
type Plan struct {
	Mode         Mode
	Added        []string
	Overwritten  []string
	Removed      []string
	ResultPaths  []string
	RegularFiles int
}

// Outcome identifies how a valid invocation ended at this stage.
type Outcome uint8

const (
	OutcomePlanOnly Outcome = iota
	OutcomeApplied
)

// Run always completes planning first. Authorization is checked only after
// planning, and an unauthorized invocation never calls apply. The callbacks
// deliberately do not receive authorization as a planner input.
func Run(authorized bool, plan func() (Plan, error), apply func(Plan) error) (Plan, Outcome, error) {
	computedPlan, err := plan()
	if err != nil {
		return Plan{}, OutcomePlanOnly, err
	}
	if !authorized {
		return computedPlan, OutcomePlanOnly, nil
	}
	if err := apply(computedPlan); err != nil {
		return computedPlan, OutcomeApplied, err
	}
	return computedPlan, OutcomeApplied, nil
}
