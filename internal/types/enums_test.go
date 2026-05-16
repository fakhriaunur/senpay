package types

import (
	"testing"
)

func TestParseVAStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    VAStatus
		wantErr bool
	}{
		{name: "active", input: "active", want: VAStatusActive, wantErr: false},
		{name: "paid", input: "paid", want: VAStatusPaid, wantErr: false},
		{name: "expired", input: "expired", want: VAStatusExpired, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
		{name: "typo", input: "activ", wantErr: true},
		{name: "case_sensitive", input: "ACTIVE", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVAStatus(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseVAStatus(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseVAStatus(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseVAStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if got.String() != tt.input {
				t.Errorf("VAStatus.String() = %q, want %q", got.String(), tt.input)
			}
		})
	}
}

func TestParseCallbackStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    CallbackStatus
		wantErr bool
	}{
		{name: "success", input: "success", want: CallbackSuccess, wantErr: false},
		{name: "failed", input: "failed", want: CallbackFailed, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCallbackStatus(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCallbackStatus(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseCallbackStatus(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseCallbackStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBankProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    BankProvider
		wantErr bool
	}{
		{name: "stub", input: "stub", want: BankProviderStub, wantErr: false},
		{name: "snap", input: "snap", want: BankProviderSnap, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBankProvider(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseBankProvider(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseBankProvider(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseBankProvider(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseKYCLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    KYCLevel
		wantErr bool
	}{
		{name: "basic", input: "basic", want: KYCLevelBasic, wantErr: false},
		{name: "verified", input: "verified", want: KYCLevelVerified, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
		{name: "typo", input: "verifed", wantErr: true},
		{name: "case_sensitive", input: "BASIC", wantErr: true},
		{name: "premium", input: "premium", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKYCLevel(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseKYCLevel(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseKYCLevel(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseKYCLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if got.String() != tt.input {
				t.Errorf("KYCLevel.String() = %q, want %q", got.String(), tt.input)
			}
		})
	}
}

func TestParseEntryType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    EntryType
		wantErr bool
	}{
		{name: "debit", input: "debit", want: EntryTypeDebit, wantErr: false},
		{name: "credit", input: "credit", want: EntryTypeCredit, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEntryType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseEntryType(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseEntryType(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseEntryType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTxType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    TxType
		wantErr bool
	}{
		{name: "topup", input: "topup", want: TxTypeTopup, wantErr: false},
		{name: "transfer", input: "transfer", want: TxTypeTransfer, wantErr: false},
		{name: "withdraw", input: "withdraw", want: TxTypeWithdraw, wantErr: false},
		{name: "fee", input: "fee", want: TxTypeFee, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
		{name: "typo", input: "transfir", wantErr: true},
		{name: "case_sensitive", input: "TOPUP", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTxType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTxType(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseTxType(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTxType(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if got.String() != tt.input {
				t.Errorf("TxType.String() = %q, want %q", got.String(), tt.input)
			}
		})
	}
}

func TestParseTxStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    TxStatus
		wantErr bool
	}{
		{name: "pending", input: "pending", want: TxStatusPending, wantErr: false},
		{name: "committed", input: "committed", want: TxStatusCommitted, wantErr: false},
		{name: "failed", input: "failed", want: TxStatusFailed, wantErr: false},
		{name: "compensated", input: "compensated", want: TxStatusCompensated, wantErr: false},
		{name: "timeout", input: "timeout", want: TxStatusTimeout, wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
		{name: "typo", input: "pendng", wantErr: true},
		{name: "case_sensitive", input: "COMMITTED", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTxStatus(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTxStatus(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseTxStatus(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTxStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if got.String() != tt.input {
				t.Errorf("TxStatus.String() = %q, want %q", got.String(), tt.input)
			}
		})
	}
}
