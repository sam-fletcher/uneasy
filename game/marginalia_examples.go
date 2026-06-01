package game

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed marginalia_examples.json
var marginaliaExamplesJSON []byte

// MarginaliaExamples maps an asset type ("peer"/"holding"/"artifact"/"resource")
// to a list of suggested marginalia. These are generic, type-keyed inspiration
// shown when a player adds a marginalium from scratch — distinct from
// PrologueExamples, which suggests asset *names*. Loaded once from the embedded
// JSON at startup.
var MarginaliaExamples = mustLoadMarginaliaExamples()

func mustLoadMarginaliaExamples() map[string][]string {
	var out map[string][]string
	if err := json.Unmarshal(marginaliaExamplesJSON, &out); err != nil {
		panic(fmt.Sprintf("marginalia_examples.json: %v", err))
	}
	return out
}
