package fee

import (
	"testing"
	"time"

	"senpay/internal/types"
)

// defaultPromoConfig is a test promo config matching the production fees.yaml values.
var defaultPromoConfig = PromoConfig{
	DiscountPct: 100.0,
	FreeTransferWindow: Window{
		StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
	},
	CampaignCodes: []string{"BEBASFEE", "GRATIS-ONGKIR"},
}

// withinWindow is a test time guaranteed to be within the default window.
var withinWindow = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// outsideWindow is a test time guaranteed to be outside the default window.
var outsideWindow = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

func TestPromoService_ApplyPromoCode_NoCode(t *testing.T) {
	t.Parallel()

	svc := NewPromoService(defaultPromoConfig)
	result := svc.ApplyPromoCode("", types.Money(50000), withinWindow)

	if result.PromoWarning != nil {
		t.Errorf("expected no warning for empty code, got %v", result.PromoWarning)
	}
	if result.DiscountedFee != 50000 {
		t.Errorf("expected fee unchanged (50000), got %d", result.DiscountedFee)
	}
	if result.Applied {
		t.Error("expected Applied=false for empty code")
	}
}

func TestPromoService_ApplyPromoCode_InvalidFormat(t *testing.T) {
	t.Parallel()

	svc := NewPromoService(defaultPromoConfig)
	result := svc.ApplyPromoCode("INVALID!!!", types.Money(50000), withinWindow)

	if result.PromoWarning != nil {
		if result.PromoWarning.Code != types.ErrCodePromoInvalid {
			t.Errorf("expected PromoInvalid error code, got %q", result.PromoWarning.Code)
		}
	} else {
		t.Error("expected PromoWarning for invalid format")
	}
	if result.DiscountedFee != 50000 {
		t.Errorf("expected fee unchanged (50000), got %d", result.DiscountedFee)
	}
	if result.Applied {
		t.Error("expected Applied=false for invalid format")
	}
}

func TestPromoService_ApplyPromoCode_UnknownCode(t *testing.T) {
	t.Parallel()

	svc := NewPromoService(defaultPromoConfig)
	result := svc.ApplyPromoCode("UNKNOWN-CODE", types.Money(50000), withinWindow)

	if result.PromoWarning != nil {
		if result.PromoWarning.Code != types.ErrCodePromoInvalid {
			t.Errorf("expected PromoInvalid error code, got %q", result.PromoWarning.Code)
		}
	} else {
		t.Error("expected PromoWarning for unknown code")
	}
	if result.DiscountedFee != 50000 {
		t.Errorf("expected fee unchanged (50000), got %d", result.DiscountedFee)
	}
	if result.Applied {
		t.Error("expected Applied=false for unknown code")
	}
}

func TestPromoService_ApplyPromoCode_Expired(t *testing.T) {
	t.Parallel()

	svc := NewPromoService(defaultPromoConfig)
	// Use a time outside the free transfer window.
	result := svc.ApplyPromoCode("BEBASFEE", types.Money(50000), outsideWindow)

	if result.PromoWarning != nil {
		if result.PromoWarning.Code != types.ErrCodePromoExpired {
			t.Errorf("expected PromoExpired error code, got %q", result.PromoWarning.Code)
		}
	} else {
		t.Error("expected PromoWarning for expired promo")
	}
	if result.DiscountedFee != 50000 {
		t.Errorf("expected fee unchanged (50000), got %d", result.DiscountedFee)
	}
	if result.Applied {
		t.Error("expected Applied=false for expired promo")
	}
}

func TestPromoService_ApplyPromoCode_Success100Percent(t *testing.T) {
	t.Parallel()

	svc := NewPromoService(defaultPromoConfig)
	result := svc.ApplyPromoCode("BEBASFEE", types.Money(2500), withinWindow)

	if result.PromoWarning != nil {
		t.Errorf("expected no warning for valid promo, got %v", result.PromoWarning)
	}
	if result.DiscountedFee != 0 {
		t.Errorf("expected discounted fee 0 for 100%% discount, got %d", result.DiscountedFee)
	}
	if !result.Applied {
		t.Error("expected Applied=true for valid promo")
	}
}

func TestPromoService_ApplyPromoCode_SuccessWithHyphen(t *testing.T) {
	t.Parallel()

	svc := NewPromoService(defaultPromoConfig)
	result := svc.ApplyPromoCode("GRATIS-ONGKIR", types.Money(2500), withinWindow)

	if result.PromoWarning != nil {
		t.Errorf("expected no warning for valid promo with hyphen, got %v", result.PromoWarning)
	}
	if result.DiscountedFee != 0 {
		t.Errorf("expected discounted fee 0 for 100%% discount, got %d", result.DiscountedFee)
	}
	if !result.Applied {
		t.Error("expected Applied=true for valid promo")
	}
}

func TestPromoService_ApplyPromoCode_PartialDiscount(t *testing.T) {
	t.Parallel()

	cfg := PromoConfig{
		DiscountPct: 50.0,
		FreeTransferWindow: Window{
			StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
		},
		CampaignCodes: []string{"HALF-OFF"},
	}
	svc := NewPromoService(cfg)
	result := svc.ApplyPromoCode("HALF-OFF", types.Money(10000), withinWindow)

	if result.PromoWarning != nil {
		t.Errorf("expected no warning, got %v", result.PromoWarning)
	}
	if result.DiscountedFee != 5000 {
		t.Errorf("expected discounted fee 5000 (50%% of 10000), got %d", result.DiscountedFee)
	}
	if !result.Applied {
		t.Error("expected Applied=true")
	}
}

func TestPromoService_ApplyPromoCode_ZeroDiscount(t *testing.T) {
	t.Parallel()

	cfg := PromoConfig{
		DiscountPct: 0.0,
		FreeTransferWindow: Window{
			StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
		},
		CampaignCodes: []string{"NO-DISCOUNT"},
	}
	svc := NewPromoService(cfg)
	result := svc.ApplyPromoCode("NO-DISCOUNT", types.Money(50000), withinWindow)

	if result.PromoWarning != nil {
		t.Errorf("expected no warning, got %v", result.PromoWarning)
	}
	if result.DiscountedFee != 50000 {
		t.Errorf("expected fee unchanged (50000) for 0%% discount, got %d", result.DiscountedFee)
	}
	if !result.Applied {
		t.Error("expected Applied=true")
	}
}

func TestWindow_Contains(t *testing.T) {
	t.Parallel()

	window := Window{
		StartTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	}

	tests := []struct {
		name string
		time time.Time
		want bool
	}{
		{name: "before_window", time: time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC), want: false},
		{name: "exactly_start", time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), want: true},
		{name: "within_window", time: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC), want: true},
		{name: "exactly_end", time: time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC), want: true},
		{name: "after_window", time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := window.Contains(tt.time)
			if got != tt.want {
				t.Errorf("Window.Contains(%v) = %v, want %v", tt.time, got, tt.want)
			}
		})
	}
}

func TestPromoService_ApplyPromoCode_WindowBoundaries(t *testing.T) {
	t.Parallel()

	window := Window{
		StartTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	}
	cfg := PromoConfig{
		DiscountPct:          100.0,
		FreeTransferWindow:   window,
		CampaignCodes:        []string{"BEBASFEE"},
	}
	svc := NewPromoService(cfg)

	// Exactly at start time — should be valid.
	result := svc.ApplyPromoCode("BEBASFEE", types.Money(2500), window.StartTime)
	if !result.Applied {
		t.Error("expected promo to apply exactly at window start time")
	}
	if result.DiscountedFee != 0 {
		t.Errorf("expected 0 fee at start boundary, got %d", result.DiscountedFee)
	}

	// Exactly at end time — should be valid.
	result = svc.ApplyPromoCode("BEBASFEE", types.Money(2500), window.EndTime)
	if !result.Applied {
		t.Error("expected promo to apply exactly at window end time")
	}
	if result.DiscountedFee != 0 {
		t.Errorf("expected 0 fee at end boundary, got %d", result.DiscountedFee)
	}

	// One second after end — should be expired.
	afterEnd := window.EndTime.Add(1 * time.Second)
	result = svc.ApplyPromoCode("BEBASFEE", types.Money(2500), afterEnd)
	if result.Applied {
		t.Error("expected promo NOT to apply after window end")
	}
	if result.PromoWarning == nil || result.PromoWarning.Code != types.ErrCodePromoExpired {
		t.Errorf("expected PromoExpired error, got %v", result.PromoWarning)
	}
}
