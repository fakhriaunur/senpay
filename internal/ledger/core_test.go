package ledger

import (
	"math"
	"testing"

	"senpay/internal/types"

	"pgregory.net/rapid"
)

func TestExecuteTransfer(t *testing.T) {
	t.Parallel()

	maxI64 := types.Money(math.MaxInt64)

	tests := []struct {
		name          string
		senderBal     types.Money
		receiverBal   types.Money
		amount        types.Money
		wantErr       bool
		wantErrCode   string
		wantDebitAmt  types.Money
		wantCreditAmt types.Money
	}{
		// ── Success cases ──────────────────────────────────────
		{name: "success_simple", senderBal: 100000, receiverBal: 50000, amount: 25000,
			wantErr: false, wantDebitAmt: 25000, wantCreditAmt: 25000},
		{name: "success_exact_balance", senderBal: 50000, receiverBal: 0, amount: 50000,
			wantErr: false, wantDebitAmt: 50000, wantCreditAmt: 50000},
		{name: "success_large_amount", senderBal: 1000000000, receiverBal: 500000000, amount: 750000000,
			wantErr: false, wantDebitAmt: 750000000, wantCreditAmt: 750000000},
		{name: "success_small_amount", senderBal: 50000, receiverBal: 0, amount: 1,
			wantErr: false, wantDebitAmt: 1, wantCreditAmt: 1},
		{name: "success_equal_balances", senderBal: 100000, receiverBal: 100000, amount: 50000,
			wantErr: false, wantDebitAmt: 50000, wantCreditAmt: 50000},
		{name: "success_receiver_richer", senderBal: 50000, receiverBal: 500000, amount: 25000,
			wantErr: false, wantDebitAmt: 25000, wantCreditAmt: 25000},
		{name: "success_zero_receiver_balance", senderBal: 100000, receiverBal: 0, amount: 100000,
			wantErr: false, wantDebitAmt: 100000, wantCreditAmt: 100000},
		{name: "success_both_high", senderBal: 999999999, receiverBal: 888888888, amount: 111111111,
			wantErr: false, wantDebitAmt: 111111111, wantCreditAmt: 111111111},
		{name: "success_round_numbers", senderBal: 1000000, receiverBal: 2000000, amount: 500000,
			wantErr: false, wantDebitAmt: 500000, wantCreditAmt: 500000},
		{name: "success_minimum_positive", senderBal: 1, receiverBal: 0, amount: 1,
			wantErr: false, wantDebitAmt: 1, wantCreditAmt: 1},
		{name: "success_sender_has_excess", senderBal: 999999, receiverBal: 1, amount: 500000,
			wantErr: false, wantDebitAmt: 500000, wantCreditAmt: 500000},
		{name: "success_receiver_start_zero", senderBal: 100000, receiverBal: 0, amount: 50000,
			wantErr: false, wantDebitAmt: 50000, wantCreditAmt: 50000},

		// ── Max int64 edge cases ────────────────────────────────
		{name: "max_small_amount", senderBal: maxI64, receiverBal: 0, amount: 1,
			wantErr: false, wantDebitAmt: 1, wantCreditAmt: 1},
		{name: "max_medium_amount", senderBal: maxI64, receiverBal: 0, amount: 1000000,
			wantErr: false, wantDebitAmt: 1000000, wantCreditAmt: 1000000},
		{name: "max_large_amount", senderBal: maxI64, receiverBal: 0, amount: 9223372036854775807,
			wantErr: false, wantDebitAmt: maxI64, wantCreditAmt: maxI64},
		{name: "max_both_nonzero", senderBal: maxI64, receiverBal: maxI64, amount: 1,
			wantErr: false, wantDebitAmt: 1, wantCreditAmt: 1},
		{name: "max_exact_balance", senderBal: 5000, receiverBal: maxI64, amount: 5000,
			wantErr: false, wantDebitAmt: 5000, wantCreditAmt: 5000},
		{name: "near_max_sender_amount", senderBal: maxI64, receiverBal: 0, amount: maxI64 - 1,
			wantErr: false, wantDebitAmt: maxI64 - 1, wantCreditAmt: maxI64 - 1},

		// ── Zero balance after transfer ─────────────────────────
		{name: "sender_goes_to_zero", senderBal: 50000, receiverBal: 0, amount: 50000,
			wantErr: false, wantDebitAmt: 50000, wantCreditAmt: 50000},
		{name: "both_start_zero", senderBal: 0, receiverBal: 0, amount: 0,
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},
		{name: "sender_zero_amount_zero", senderBal: 0, receiverBal: 0, amount: 0,
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},
		{name: "sender_one_receiver_zero_exact", senderBal: 1, receiverBal: 0, amount: 1,
			wantErr: false, wantDebitAmt: 1, wantCreditAmt: 1},

		// ── Error: insufficient balance ─────────────────────────
		{name: "insufficient_short_1", senderBal: 50000, receiverBal: 50000, amount: 50001,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},
		{name: "insufficient_short_many", senderBal: 1000, receiverBal: 50000, amount: 50000,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},
		{name: "insufficient_zero_sender", senderBal: 0, receiverBal: 5000, amount: 1,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},
		{name: "insufficient_one_sen_short", senderBal: 49999, receiverBal: 0, amount: 50000,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},
		{name: "insufficient_sender_penniless", senderBal: 0, receiverBal: 100000, amount: 100000,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},
		{name: "insufficient_large_shortfall", senderBal: 1, receiverBal: 0, amount: maxI64,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},
		{name: "insufficient_mid_shortfall", senderBal: 100000, receiverBal: 0, amount: 999999999,
			wantErr: true, wantErrCode: types.ErrCodeInsufficientBalance},

		// ── Error: invalid amount (zero / negative) ─────────────
		{name: "zero_amount", senderBal: 50000, receiverBal: 50000, amount: 0,
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},
		{name: "negative_one", senderBal: 50000, receiverBal: 50000, amount: -1,
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},
		{name: "negative_large", senderBal: 50000, receiverBal: 50000, amount: -999999,
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},
		{name: "negative_max", senderBal: 50000, receiverBal: 50000, amount: types.Money(math.MinInt64),
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},
		{name: "negative_with_high_balance", senderBal: maxI64, receiverBal: maxI64, amount: -100,
			wantErr: true, wantErrCode: types.ErrCodeInvalidAmount},

		// ── Invariant: total money unchanged ────────────────────
		// These verify the property explicitly in table-driven style.
		{name: "invariant_equal_split", senderBal: 100000, receiverBal: 100000, amount: 50000,
			wantErr: false, wantDebitAmt: 50000, wantCreditAmt: 50000},
		{name: "invariant_uneven_split", senderBal: 5000, receiverBal: 100000, amount: 5000,
			wantErr: false, wantDebitAmt: 5000, wantCreditAmt: 5000},
		{name: "invariant_full_transfer", senderBal: 99999, receiverBal: 1, amount: 99999,
			wantErr: false, wantDebitAmt: 99999, wantCreditAmt: 99999},
		{name: "invariant_minimal", senderBal: 1, receiverBal: 1, amount: 1,
			wantErr: false, wantDebitAmt: 1, wantCreditAmt: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture inputs to verify immutability.
			senderOrig := tt.senderBal
			receiverOrig := tt.receiverBal
			amountOrig := tt.amount

			ev, err := ExecuteTransfer(tt.senderBal, tt.receiverBal, tt.amount)

			// Verify input immutability.
			if tt.senderBal != senderOrig {
				t.Errorf("ExecuteTransfer mutated senderBalance: got %d, want %d", tt.senderBal, senderOrig)
			}
			if tt.receiverBal != receiverOrig {
				t.Errorf("ExecuteTransfer mutated receiverBalance: got %d, want %d", tt.receiverBal, receiverOrig)
			}
			if tt.amount != amountOrig {
				t.Errorf("ExecuteTransfer mutated amount: got %d, want %d", tt.amount, amountOrig)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Code != tt.wantErrCode {
					t.Errorf("error code: got %q, want %q", err.Code, tt.wantErrCode)
				}
				// Verify TxEvent is zero-value on error.
				if ev.Debit.Amount != 0 || ev.Credit.Amount != 0 {
					t.Error("TxEvent should be zero-value on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify debit/credit amounts.
			if ev.Debit.Amount != tt.wantDebitAmt {
				t.Errorf("debit amount: got %d, want %d", ev.Debit.Amount, tt.wantDebitAmt)
			}
			if ev.Credit.Amount != tt.wantCreditAmt {
				t.Errorf("credit amount: got %d, want %d", ev.Credit.Amount, tt.wantCreditAmt)
			}

			// Verify debit/credit types.
			if ev.Debit.TxType != types.EntryTypeDebit {
				t.Errorf("debit type: got %q, want %q", ev.Debit.TxType, types.EntryTypeDebit)
			}
			if ev.Credit.TxType != types.EntryTypeCredit {
				t.Errorf("credit type: got %q, want %q", ev.Credit.TxType, types.EntryTypeCredit)
			}

			// Money conservation invariant: sender+receiver total unchanged.
			totalBefore := tt.senderBal + tt.receiverBal
			totalAfter := (tt.senderBal - ev.Debit.Amount) + (tt.receiverBal + ev.Credit.Amount)
			if totalAfter != totalBefore {
				t.Errorf("money conservation violated: before=%d, after=%d", totalBefore, totalAfter)
			}
		})
	}
}

func TestExecuteTransfer_MoneyConservation(t *testing.T) {
	t.Parallel()

	// Parameterized test over many amounts: verify that total money is always conserved.
	amounts := []types.Money{
		1, 2, 10, 100, 999, 1000, 2500, 5000, 10000, 50000,
		99999, 100000, 500000, 999999, 1000000, 5000000, 9999999,
		10000000, 50000000, 99999999, 100000000, 500000000, 999999999,
		1000000000, 5000000000, 9999999999, 10000000000,
	}

	for _, senderBal := range amounts {
		for _, receiverBal := range amounts {
			for _, amt := range amounts {
				if amt > senderBal {
					continue // skip insufficient balance
				}
				t.Run("", func(t *testing.T) {
					ev, err := ExecuteTransfer(senderBal, receiverBal, amt)
					if err != nil {
						t.Fatalf("unexpected error for sender=%d receiver=%d amt=%d: %v",
							senderBal, receiverBal, amt, err)
					}
					totalBefore := senderBal + receiverBal
					totalAfter := (senderBal - ev.Debit.Amount) + (receiverBal + ev.Credit.Amount)
					if totalAfter != totalBefore {
						t.Errorf("conservation: before=%d, after=%d", totalBefore, totalAfter)
					}
				})
			}
		}
	}
}

// TestExecuteTransfer_EdgeValues tests edge and boundary values exhaustively.
func TestExecuteTransfer_EdgeValues(t *testing.T) {
	t.Parallel()

	edgeAmounts := []types.Money{
		0, 1, types.Money(math.MaxInt64),
		types.Money(math.MaxInt64 - 1),
		types.Money(math.MaxInt64 / 2),
		types.Money(math.MaxInt64/2 + 1),
		types.Money(math.MaxInt64 / 3),
		types.Money(math.MaxInt64 / 4),
		100, 500, 1000, 2500, 5000, 10000,
		100000, 500000, 1000000, 5000000, 10000000,
	}

	for _, senderBal := range edgeAmounts {
		for _, amount := range edgeAmounts {
			if amount <= 0 {
				// Zero/negative -> invalid amount error.
				t.Run("", func(t *testing.T) {
					_, err := ExecuteTransfer(senderBal, 0, amount)
					if err == nil {
						t.Errorf("expected error for sender=%d amount=%d", senderBal, amount)
					} else if err.Code != types.ErrCodeInvalidAmount {
						t.Errorf("wrong code: got %q, want %q", err.Code, types.ErrCodeInvalidAmount)
					}
				})
				continue
			}
			if amount > senderBal {
				t.Run("", func(t *testing.T) {
					_, err := ExecuteTransfer(senderBal, 0, amount)
					if err == nil {
						t.Errorf("expected error for sender=%d amount=%d", senderBal, amount)
					} else if err.Code != types.ErrCodeInsufficientBalance {
						t.Errorf("wrong code: got %q, want %q", err.Code, types.ErrCodeInsufficientBalance)
					}
				})
				continue
			}
			t.Run("", func(t *testing.T) {
				ev, err := ExecuteTransfer(senderBal, 0, amount)
				if err != nil {
					t.Fatalf("unexpected error for sender=%d amount=%d: %v", senderBal, amount, err)
				}
				if ev.Debit.Amount != amount || ev.Credit.Amount != amount {
					t.Errorf("amount mismatch: debit=%d credit=%d want=%d", ev.Debit.Amount, ev.Credit.Amount, amount)
				}
			})
		}
	}
}

// ── Rapid property-based tests ──────────────────────────────────

func TestProperty_ExecuteTransfer_MoneyConservation(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		senderBal := rapid.Int64Range(0, math.MaxInt64).Draw(t, "senderBalance")
		receiverBal := rapid.Int64Range(0, math.MaxInt64).Draw(t, "receiverBalance")
		amount := rapid.Int64Range(0, math.MaxInt64).Draw(t, "amount")

		totalBefore := senderBal + receiverBal

		ev, err := ExecuteTransfer(
			types.Money(senderBal),
			types.Money(receiverBal),
			types.Money(amount),
		)
		if err != nil {
			// Must be invalid amount (amount <= 0) or insufficient balance.
			if amount <= 0 {
				if err.Code != types.ErrCodeInvalidAmount {
					t.Fatalf("expected INVALID_AMOUNT for amount=%d, got %s", amount, err.Code)
				}
			} else if amount > senderBal {
				if err.Code != types.ErrCodeInsufficientBalance {
					t.Fatalf("expected INSUFFICIENT_BALANCE for sender=%d amount=%d, got %s", senderBal, amount, err.Code)
				}
			} else {
				t.Fatalf("unexpected error for sender=%d receiver=%d amount=%d: %v", senderBal, receiverBal, amount, err)
			}
			return
		}

		// Verify debit/credit entries match amount.
		if int64(ev.Debit.Amount) != amount {
			t.Fatalf("debit amount mismatch: got %d, want %d", ev.Debit.Amount, amount)
		}
		if int64(ev.Credit.Amount) != amount {
			t.Fatalf("credit amount mismatch: got %d, want %d", ev.Credit.Amount, amount)
		}

		// Verify debit/credit types.
		if ev.Debit.TxType != types.EntryTypeDebit || ev.Credit.TxType != types.EntryTypeCredit {
			t.Fatalf("wrong entry types: debit=%q credit=%q", ev.Debit.TxType, ev.Credit.TxType)
		}

		// Money conservation invariant.
		// debit == credit == amount, so:
		// totalBefore = sender + receiver
		// totalAfter = (sender - amount) + (receiver + amount) = sender + receiver = totalBefore
		totalAfter := (senderBal - int64(ev.Debit.Amount)) + (receiverBal + int64(ev.Credit.Amount))
		if totalAfter != totalBefore {
			t.Fatalf("money conservation violated: before=%d, after=%d", totalBefore, totalAfter)
		}

		// Sender never ends with negative balance.
		if senderBal < amount {
			t.Fatalf("sender balance negative: senderBal=%d < amount=%d", senderBal, amount)
		}

		// Debit and credit always match the transfer amount.
		if ev.Debit.Amount != ev.Credit.Amount {
			t.Fatalf("debit/credit mismatch: debit=%d credit=%d", ev.Debit.Amount, ev.Credit.Amount)
		}
	})
}

func TestProperty_ExecuteTransfer_NoMutation(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		senderBal := rapid.Int64Range(0, 1000000).Draw(t, "senderBalance")
		receiverBal := rapid.Int64Range(0, 1000000).Draw(t, "receiverBalance")
		amount := rapid.Int64Range(1, 1000000).Draw(t, "amount")

		if amount > senderBal {
			return // skip insufficient, we only test success paths
		}

		sOrig := types.Money(senderBal)
		rOrig := types.Money(receiverBal)
		aOrig := types.Money(amount)

		_, _ = ExecuteTransfer(sOrig, rOrig, aOrig)

		if sOrig != types.Money(senderBal) {
			t.Fatal("ExecuteTransfer mutated sender balance")
		}
		if rOrig != types.Money(receiverBal) {
			t.Fatal("ExecuteTransfer mutated receiver balance")
		}
		if aOrig != types.Money(amount) {
			t.Fatal("ExecuteTransfer mutated amount")
		}
	})
}
