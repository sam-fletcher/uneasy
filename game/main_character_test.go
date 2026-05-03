package game

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

func peer(id int64) *dbgen.Asset {
	return &dbgen.Asset{ID: id, AssetType: model.AssetPeer}
}

func nonPeer(id int64, t model.AssetType) *dbgen.Asset {
	return &dbgen.Asset{ID: id, AssetType: t}
}

func mar(pos int16, torn bool) dbgen.Marginalium {
	return dbgen.Marginalium{Position: pos, Text: "x", IsTorn: torn}
}

func TestDecideMainCharacterChange(t *testing.T) {
	tests := []struct {
		name           string
		target         *dbgen.Asset
		oldMC          *dbgen.Asset
		oldMCMargs     []dbgen.Marginalium
		tearPos        *int16
		wantErr        bool
		wantErrCode    int
		wantNeedsTear  bool
		wantTearPos    int16
		wantDestroysMC bool
	}{
		{
			name:        "non-peer rejected",
			target:      nonPeer(1, model.AssetHolding),
			wantErr:     true,
			wantErrCode: http.StatusBadRequest,
		},
		{
			name:   "no prior MC → straight flip, no tear",
			target: peer(1),
		},
		{
			name:       "old MC has all torn → no tear required",
			target:     peer(1),
			oldMC:      peer(2),
			oldMCMargs: []dbgen.Marginalium{mar(1, true), mar(2, true)},
		},
		{
			name:        "old MC has untorn, missing tear_position → 400",
			target:      peer(1),
			oldMC:       peer(2),
			oldMCMargs:  []dbgen.Marginalium{mar(1, false)},
			wantErr:     true,
			wantErrCode: http.StatusBadRequest,
		},
		{
			name:        "tear_position out of range (low)",
			target:      peer(1),
			oldMC:       peer(2),
			oldMCMargs:  []dbgen.Marginalium{mar(1, false)},
			tearPos:     new(int16(0)),
			wantErr:     true,
			wantErrCode: http.StatusBadRequest,
		},
		{
			name:        "tear_position out of range (high)",
			target:      peer(1),
			oldMC:       peer(2),
			oldMCMargs:  []dbgen.Marginalium{mar(1, false)},
			tearPos:     new(int16(5)),
			wantErr:     true,
			wantErrCode: http.StatusBadRequest,
		},
		{
			name:        "tear_position points at no marginalia",
			target:      peer(1),
			oldMC:       peer(2),
			oldMCMargs:  []dbgen.Marginalium{mar(1, false)},
			tearPos:     new(int16(3)),
			wantErr:     true,
			wantErrCode: http.StatusBadRequest,
		},
		{
			name:        "tear_position points at already-torn marginalia",
			target:      peer(1),
			oldMC:       peer(2),
			oldMCMargs:  []dbgen.Marginalium{mar(1, true), mar(2, false)},
			tearPos:     new(int16(1)),
			wantErr:     true,
			wantErrCode: http.StatusBadRequest,
		},
		{
			name:          "happy path → tear, do not destroy",
			target:        peer(1),
			oldMC:         peer(2),
			oldMCMargs:    []dbgen.Marginalium{mar(1, false), mar(2, false), mar(3, true)},
			tearPos:       new(int16(2)),
			wantNeedsTear: true,
			wantTearPos:   2,
		},
		{
			name:           "tearing the last untorn → destroys old MC",
			target:         peer(1),
			oldMC:          peer(2),
			oldMCMargs:     []dbgen.Marginalium{mar(1, true), mar(2, false), mar(3, true), mar(4, true)},
			tearPos:        new(int16(2)),
			wantNeedsTear:  true,
			wantTearPos:    2,
			wantDestroysMC: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecideMainCharacterChange(tt.target, tt.oldMC, tt.oldMCMargs, tt.tearPos)
			if tt.wantErr {
				require.NotNil(t, err)
				assert.Equal(t, tt.wantErrCode, err.Code)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, tt.wantNeedsTear, got.NeedsTear)
			if got.NeedsTear {
				assert.Equal(t, tt.wantTearPos, got.TearPosition)
			}
			assert.Equal(t, tt.wantDestroysMC, got.DestroysOldMC)
		})
	}
}

func TestDecideMainCharacterChange_NilTarget(t *testing.T) {
	_, err := DecideMainCharacterChange(nil, nil, nil, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, err.Code)
}
