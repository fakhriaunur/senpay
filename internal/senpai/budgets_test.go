package senpai

import (
	"testing"
)

func TestBudgetWithAlert_ComputeAlert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		spentSen  int64
		limitSen  int64
		wantAlert bool
		wantExceeded bool
		wantPct   float64
	}{
		{
			name:      "under 80%",
			spentSen:  300000,
			limitSen:  500000,
			wantAlert: false,
			wantExceeded: false,
			wantPct:   60.0,
		},
		{
			name:      "exactly 80%",
			spentSen:  400000,
			limitSen:  500000,
			wantAlert: true,
			wantExceeded: false,
			wantPct:   80.0,
		},
		{
			name:      "above 80% but under 100%",
			spentSen:  450000,
			limitSen:  500000,
			wantAlert: true,
			wantExceeded: false,
			wantPct:   90.0,
		},
		{
			name:      "exactly 100%",
			spentSen:  500000,
			limitSen:  500000,
			wantAlert: true,
			wantExceeded: true,
			wantPct:   100.0,
		},
		{
			name:      "over 100%",
			spentSen:  600000,
			limitSen:  500000,
			wantAlert: true,
			wantExceeded: true,
			wantPct:   120.0,
		},
		{
			name:      "zero spending",
			spentSen:  0,
			limitSen:  500000,
			wantAlert: false,
			wantExceeded: false,
			wantPct:   0.0,
		},
		{
			name:      "zero limit returns no alert",
			spentSen:  100000,
			limitSen:  0,
			wantAlert: false,
			wantExceeded: false,
			wantPct:   0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BudgetWithAlert{
				Budget: Budget{
					SpentSen: tt.spentSen,
					LimitSen: tt.limitSen,
				},
			}
			b.computeAlert()

			if b.Alert != tt.wantAlert {
				t.Errorf("Alert = %v, want %v", b.Alert, tt.wantAlert)
			}
			if b.Exceeded != tt.wantExceeded {
				t.Errorf("Exceeded = %v, want %v", b.Exceeded, tt.wantExceeded)
			}
			if tt.wantAlert && b.WarningMsg == "" {
				t.Error("expected warning message when alert is true, got empty")
			}
			if !tt.wantAlert && b.WarningMsg != "" {
				t.Errorf("expected no warning message, got %q", b.WarningMsg)
			}
			if tt.wantExceeded && b.WarningMsg == "" {
				t.Error("expected warning message when exceeded, got empty")
			}
		})
	}
}
