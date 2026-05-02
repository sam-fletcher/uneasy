package game

import (
	"net/http"
	"testing"

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
				if err == nil {
					t.Fatalf("expected error, got decision %+v", got)
				}
				if err.Code != tt.wantErrCode {
					t.Errorf("error code = %d, want %d (msg=%q)", err.Code, tt.wantErrCode, err.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %+v", err)
			}
			if got.NeedsTear != tt.wantNeedsTear {
				t.Errorf("NeedsTear = %v, want %v", got.NeedsTear, tt.wantNeedsTear)
			}
			if got.NeedsTear && got.TearPosition != tt.wantTearPos {
				t.Errorf("TearPosition = %d, want %d", got.TearPosition, tt.wantTearPos)
			}
			if got.DestroysOldMC != tt.wantDestroysMC {
				t.Errorf("DestroysOldMC = %v, want %v", got.DestroysOldMC, tt.wantDestroysMC)
			}
		})
	}
}

func TestDecideMainCharacterChange_NilTarget(t *testing.T) {
	_, err := DecideMainCharacterChange(nil, nil, nil, nil)
	if err == nil || err.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 error for nil target, got %+v", err)
	}
}
