package game

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed prologue_examples.json
var prologueExamplesJSON []byte

// PrologueExamples maps an asset type ("peer"/"holding"/"artifact"/"resource")
// to a list of suggested names. Loaded once from the embedded JSON file at
// startup; editable without recompiling? No — embed bakes the file in. The
// JSON path lets a maintainer expand the list without touching Go code.
var PrologueExamples = mustLoadPrologueExamples()

func mustLoadPrologueExamples() map[string][]string {
	var out map[string][]string
	if err := json.Unmarshal(prologueExamplesJSON, &out); err != nil {
		panic(fmt.Sprintf("prologue_examples.json: %v", err))
	}
	return out
}
