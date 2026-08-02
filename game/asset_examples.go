package game

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed asset_examples.json
var assetExamplesJSON []byte

// AssetExamples maps an asset type ("peer"/"holding"/"artifact"/"resource")
// to a list of suggested names. Loaded once from the embedded JSON file at
// startup; editable without recompiling? No — embed bakes the file in. The
// JSON path lets a maintainer expand the list without touching Go code.
var AssetExamples = mustLoadAssetExamples()

func mustLoadAssetExamples() map[string][]string {
	var out map[string][]string
	if err := json.Unmarshal(assetExamplesJSON, &out); err != nil {
		panic(fmt.Sprintf("asset_examples.json: %v", err))
	}
	return out
}
