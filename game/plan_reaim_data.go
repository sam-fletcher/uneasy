package game

// plan_reaim_data.go — the cross-plan "this plan has lost its mark" state.
//
// It rides the top level of ResolutionData (like DemandRetargetFinalized) rather
// than a per-plan nested struct because the same state is written by three plan
// types today (Exchange Courtiers, Spread Rumors, Make Demands) and is read by
// code that should not care which: the owed-gate middleware, ComputeRowState,
// and the shared re-aim panel.

// ReAimState records that a plan's prepare-time reference stopped being valid
// before the plan could act on it, and that its preparer owes a new aim.
//
// The rulebook (p.12): "Targets are declared while preparing the plan, but
// things change. When resolving, players are free to change the target of their
// plan if the original intent no longer makes sense." A dead target is the
// clearest case of the intent no longer making sense, so the plan holds at the
// top of its resolution until the preparer names a replacement.
type ReAimState struct {
	// Owed is true from the moment the stale reference is found until the
	// preparer re-aims. While it is set every mutating plan route is refused
	// (ReAimOwedGuard) and the row-state names the preparer.
	Owed bool `json:"owed"`
	// Reason is the plain-text account of what changed ("The peer X is
	// destroyed."), written once when the gate closes so every client explains
	// the pause in the same words. Log markup never lands here.
	Reason string `json:"reason,omitempty"`
}

// ReAimOwed reports whether the plan is paused waiting for its preparer to
// re-aim. Safe on a nil receiver, so callers can chain off the optional field.
func (s *ReAimState) ReAimOwed() bool { return s != nil && s.Owed }

// EnsureReAim returns r.ReAim, allocating a zero struct if it was nil.
func (r *ResolutionData) EnsureReAim() *ReAimState {
	if r.ReAim == nil {
		r.ReAim = &ReAimState{}
	}
	return r.ReAim
}
