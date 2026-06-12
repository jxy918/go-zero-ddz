package match

import (
	"testing"
)

func TestGetTier(t *testing.T) {
	tests := []struct {
		name     string
		elo      int32
		expected string
	}{
		{"bronze I", 0, "青铜I"},
		{"bronze I max", 299, "青铜I"},
		{"bronze II", 300, "青铜II"},
		{"bronze II max", 599, "青铜II"},
		{"bronze III", 600, "青铜III"},
		{"silver I", 900, "白银I"},
		{"gold II", 2100, "黄金II"},
		{"platinum III", 3300, "铂金III"},
		{"master", 3600, "大师"},
		{"master high", 5000, "大师"},
		{"below min", -100, "青铜I"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTier(tt.elo)
			if result != tt.expected {
				t.Errorf("GetTier(%d) = %s, expected %s", tt.elo, result, tt.expected)
			}
		})
	}
}

func TestGetTierIndex(t *testing.T) {
	tests := []struct {
		name     string
		tier     string
		expected int
	}{
		{"bronze I", "青铜I", 0},
		{"bronze II", "青铜II", 1},
		{"silver I", "白银I", 3},
		{"gold III", "黄金III", 8},
		{"master", "大师", 12},
		{"unknown", "unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTierIndex(tt.tier)
			if result != tt.expected {
				t.Errorf("GetTierIndex(%s) = %d, expected %d", tt.tier, result, tt.expected)
			}
		})
	}
}

func TestCalculateELO(t *testing.T) {
	tests := []struct {
		name          string
		isWinner      bool
		isLandlord    bool
		isLandlordWin bool
		baseScore     int32
		multiplier    int32
		elo           int32
		opponentELO   int32
		wantDelta     int32
		wantPromoted  bool
	}{
		{
			name:          "landlord wins against weaker opponent",
			isWinner:      true,
			isLandlord:    true,
			isLandlordWin: true,
			baseScore:     1,
			multiplier:    1,
			elo:           1000,
			opponentELO:   800,
			wantDelta:     1,
			wantPromoted:  false,
		},
		{
			name:          "landlord wins against stronger opponent",
			isWinner:      true,
			isLandlord:    true,
			isLandlordWin: true,
			baseScore:     1,
			multiplier:    1,
			elo:           1000,
			opponentELO:   1200,
			wantDelta:     2,
			wantPromoted:  false,
		},
		{
			name:          "peasant wins",
			isWinner:      true,
			isLandlord:    false,
			isLandlordWin: false,
			baseScore:     1,
			multiplier:    1,
			elo:           1000,
			opponentELO:   1000,
			wantDelta:     1,
			wantPromoted:  false,
		},
		{
			name:          "landlord loses",
			isWinner:      false,
			isLandlord:    true,
			isLandlordWin: false,
			baseScore:     1,
			multiplier:    1,
			elo:           1000,
			opponentELO:   1000,
			wantDelta:     -2,
			wantPromoted:  false,
		},
		{
			name:          "peasant loses",
			isWinner:      false,
			isLandlord:    false,
			isLandlordWin: true,
			baseScore:     1,
			multiplier:    1,
			elo:           1000,
			opponentELO:   1000,
			wantDelta:     -1,
			wantPromoted:  false,
		},
		{
			name:          "promotion case",
			isWinner:      true,
			isLandlord:    true,
			isLandlordWin: true,
			baseScore:     10,
			multiplier:    3,
			elo:           880,
			opponentELO:   850,
			wantDelta:     60,
			wantPromoted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateELO(tt.isWinner, tt.isLandlord, tt.isLandlordWin, tt.baseScore, tt.multiplier, tt.elo, tt.opponentELO)

			if result.Delta != tt.wantDelta {
				t.Errorf("Delta = %d, want %d", result.Delta, tt.wantDelta)
			}

			if result.Promoted != tt.wantPromoted {
				t.Errorf("Promoted = %v, want %v", result.Promoted, tt.wantPromoted)
			}

			if result.OldTier != GetTier(tt.elo) {
				t.Errorf("OldTier = %s, want %s", result.OldTier, GetTier(tt.elo))
			}
		})
	}
}

func TestTiersLength(t *testing.T) {
	if len(Tiers) != 13 {
		t.Errorf("expected 13 tiers, got %d", len(Tiers))
	}
}

func TestTierRanges(t *testing.T) {
	for i, tier := range Tiers {
		if tier.Min > tier.Max {
			t.Errorf("Tier %s has invalid range: Min(%d) > Max(%d)", tier.Name, tier.Min, tier.Max)
		}
		if i > 0 && tier.Min <= Tiers[i-1].Max {
			t.Errorf("Tier %s overlaps with previous tier", tier.Name)
		}
	}
}
