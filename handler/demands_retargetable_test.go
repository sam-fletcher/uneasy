package handler

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// The list of plans a keep_or_change_target winner can act on lives twice: in
// Go (demandRetargetPlans, which gates the roll-hold and the route) and in the
// frontend (RETARGETABLE, which decides whether the form shows and whether the
// draft card is a dud). Go and TypeScript can't share a constant, so this test
// reads the TypeScript set and fails if the two differ.
func TestDemandRetargetPlans_MatchesFrontend(t *testing.T) {
	src, err := os.ReadFile("../frontend/src/lib/components/plans/shared.ts")
	require.NoError(t, err)
	m := regexp.MustCompile(`(?s)const RETARGETABLE[^=]*=\s*new Set<PlanType>\(\[(.*?)\]\)`).FindSubmatch(src)
	require.NotNil(t, m, "could not find the RETARGETABLE set in shared.ts")

	var fe []string
	for _, q := range regexp.MustCompile(`'([a-z_]+)'`).FindAllSubmatch(m[1], -1) {
		fe = append(fe, string(q[1]))
	}
	var be []string
	for _, pt := range demandRetargetPlans {
		be = append(be, string(pt))
	}
	slices.Sort(fe)
	slices.Sort(be)
	require.Equal(t, be, fe, "demandRetargetPlans (Go) and RETARGETABLE (shared.ts) must list the same plans")
}
