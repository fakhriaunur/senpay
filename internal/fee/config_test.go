package fee

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFeeConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid_yaml", func(t *testing.T) {
		content := []byte(`
flat_fee_basic_sen: 2500
rate_verified_pct: 0.7
min_fee_sen: 1000
`)
		path := writeTempYAML(t, content)
		cfg, err := LoadFeeConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.FlatFeeBasicSen != 2500 {
			t.Errorf("FlatFeeBasicSen: got %d, want 2500", cfg.FlatFeeBasicSen)
		}
		if cfg.RateVerifiedPct != 0.7 {
			t.Errorf("RateVerifiedPct: got %f, want 0.7", cfg.RateVerifiedPct)
		}
		if cfg.MinFeeSen != 1000 {
			t.Errorf("MinFeeSen: got %d, want 1000", cfg.MinFeeSen)
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := LoadFeeConfig("/nonexistent/path/fees.yaml")
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		content := []byte(`flat_fee_basic_sen: not_a_number`)
		path := writeTempYAML(t, content)
		_, err := LoadFeeConfig(path)
		if err == nil {
			t.Fatal("expected error for invalid YAML, got nil")
		}
	})

	t.Run("missing_field", func(t *testing.T) {
		// Missing flat_fee_basic_sen → zero value → validation fails.
		content := []byte(`
rate_verified_pct: 0.7
min_fee_sen: 1000
`)
		path := writeTempYAML(t, content)
		_, err := LoadFeeConfig(path)
		if err == nil {
			t.Fatal("expected error for missing flat_fee_basic_sen, got nil")
		}
	})

	t.Run("zero_flat_fee_rejected", func(t *testing.T) {
		content := []byte(`
flat_fee_basic_sen: 0
rate_verified_pct: 0.7
min_fee_sen: 1000
`)
		path := writeTempYAML(t, content)
		_, err := LoadFeeConfig(path)
		if err == nil {
			t.Fatal("expected error for zero flat_fee_basic_sen, got nil")
		}
	})

	t.Run("negative_rate_rejected", func(t *testing.T) {
		content := []byte(`
flat_fee_basic_sen: 2500
rate_verified_pct: -0.5
min_fee_sen: 1000
`)
		path := writeTempYAML(t, content)
		_, err := LoadFeeConfig(path)
		if err == nil {
			t.Fatal("expected error for negative rate_verified_pct, got nil")
		}
	})

	t.Run("zero_min_fee_rejected", func(t *testing.T) {
		content := []byte(`
flat_fee_basic_sen: 2500
rate_verified_pct: 0.7
min_fee_sen: 0
`)
		path := writeTempYAML(t, content)
		_, err := LoadFeeConfig(path)
		if err == nil {
			t.Fatal("expected error for zero min_fee_sen, got nil")
		}
	})
}

// writeTempYAML writes YAML content to a temp file and returns its path.
func writeTempYAML(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fees.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write temp YAML: %v", err)
	}
	return path
}
