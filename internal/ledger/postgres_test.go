//go:build integration

package ledger

import (
	"context"
	"sync"
	"testing"
	"time"

	"senpay/internal/store/storetest"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgres_TxLogStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer cleanup()

	store := NewPostgresTxLogStore(pool)

	// Create a test user for FK references.
	userID := createTestUser(ctx, t, pool)
	otherUserID := createTestUser(ctx, t, pool)

	t.Run("Append_and_Query", func(t *testing.T) {
		now := time.Now().UTC()
		tx := types.Transaction{
			ID:             uuid.Must(uuid.NewV7()),
			IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
			TxType:         types.TxTypeTransfer,
			SenderID:       &userID,
			ReceiverID:     &otherUserID,
			AmountSen:      50000,
			Currency:       types.CurrencyIDR,
			Status:         types.TxStatusCommitted,
			CreatedAt:      now,
			CommittedAt:    &now,
		}

		err := store.Append(ctx, tx)
		if err != nil {
			t.Fatalf("Append: %v", err)
		}

		// Query as sender.
		results, nextCursor, err := store.QueryByUserID(ctx, userID, "", 10)
		if err != nil {
			t.Fatalf("QueryByUserID: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		if results[0].ID != tx.ID {
			t.Errorf("ID: got %v, want %v", results[0].ID, tx.ID)
		}
		if results[0].AmountSen != tx.AmountSen {
			t.Errorf("AmountSen: got %d, want %d", results[0].AmountSen, tx.AmountSen)
		}
		if results[0].Status != tx.Status {
			t.Errorf("Status: got %s, want %s", results[0].Status, tx.Status)
		}
		// Query as receiver.
		results, _, err = store.QueryByUserID(ctx, otherUserID, "", 10)
		if err != nil {
			t.Fatalf("QueryByUserID (receiver): %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected receiver to see the transaction")
		}

		if nextCursor != "" {
			t.Logf("next cursor: %s", nextCursor)
		}
	})

	t.Run("Append_DuplicateIdempotencyKey", func(t *testing.T) {
		key := uuid.Must(uuid.NewV7()).String()
		now := time.Now().UTC()

		tx1 := types.Transaction{
			ID:             uuid.Must(uuid.NewV7()),
			IdempotencyKey: key,
			TxType:         types.TxTypeTransfer,
			SenderID:       &userID,
			ReceiverID:     &otherUserID,
			AmountSen:      25000,
			Currency:       types.CurrencyIDR,
			Status:         types.TxStatusCommitted,
			CreatedAt:      now,
			CommittedAt:    &now,
		}
		err := store.Append(ctx, tx1)
		if err != nil {
			t.Fatalf("Append first: %v", err)
		}

		tx2 := types.Transaction{
			ID:             uuid.Must(uuid.NewV7()),
			IdempotencyKey: key,
			TxType:         types.TxTypeTransfer,
			SenderID:       &userID,
			ReceiverID:     &otherUserID,
			AmountSen:      25000,
			Currency:       types.CurrencyIDR,
			Status:         types.TxStatusCommitted,
			CreatedAt:      now,
			CommittedAt:    &now,
		}
		err = store.Append(ctx, tx2)
		if err == nil {
			t.Fatal("expected error for duplicate idempotency key")
		}
	})

	t.Run("Query_CursorPagination", func(t *testing.T) {
		// Insert 5 transactions.
		const count = 5
		var ids []uuid.UUID
		for i := 0; i < count; i++ {
			id := uuid.Must(uuid.NewV7())
			ids = append(ids, id)
			now := time.Now().UTC()
			tx := types.Transaction{
				ID:             id,
				IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
				TxType:         types.TxTypeTransfer,
				SenderID:       &userID,
				ReceiverID:     &otherUserID,
				AmountSen:      int64(10000 * (i + 1)),
				Currency:       types.CurrencyIDR,
				Status:         types.TxStatusCommitted,
				CreatedAt:      now,
				CommittedAt:    &now,
			}
			err := store.Append(ctx, tx)
			if err != nil {
				t.Fatalf("Append %d: %v", i, err)
			}
			time.Sleep(2 * time.Millisecond) // ensure different created_at
		}

		// First page: limit 2.
		page1, cursor1, err := store.QueryByUserID(ctx, userID, "", 2)
		if err != nil {
			t.Fatalf("Query page 1: %v", err)
		}
		if len(page1) != 2 {
			t.Errorf("page 1: got %d items, want 2", len(page1))
		}
		if cursor1 == "" {
			t.Error("expected next cursor for page 1")
		}

		// Second page: limit 2.
		page2, cursor2, err := store.QueryByUserID(ctx, userID, cursor1, 2)
		if err != nil {
			t.Fatalf("Query page 2: %v", err)
		}
		if len(page2) != 2 {
			t.Errorf("page 2: got %d items, want 2", len(page2))
		}

		// Verify no overlap between pages.
		for _, p1 := range page1 {
			for _, p2 := range page2 {
				if p1.ID == p2.ID {
					t.Errorf("duplicate item across pages: %v", p1.ID)
				}
			}
		}

		// Third page: should have remaining items.
		page3, cursor3, err := store.QueryByUserID(ctx, userID, cursor2, 2)
		if err != nil {
			t.Fatalf("Query page 3: %v", err)
		}
		if len(page3) == 0 {
			t.Error("expected items on page 3")
		}
		if cursor3 != "" {
			t.Logf("page 3 has next cursor: %s", cursor3)
		}

		// Verify sorted by created_at DESC (newest first).
		allPages := append(page1, page2...)
		allPages = append(allPages, page3...)
		for i := 1; i < len(allPages); i++ {
			if allPages[i].CreatedAt.After(allPages[i-1].CreatedAt) {
				t.Errorf("items not sorted DESC at index %d: %v before %v",
					i, allPages[i].CreatedAt, allPages[i-1].CreatedAt)
			}
		}
	})

	t.Run("Append_WithAllTxTypes", func(t *testing.T) {
		txTypes := []struct {
			name    string
			txType  string
			sender  *uuid.UUID
			receiver *uuid.UUID
		}{
			{"topup", types.TxTypeTopup, nil, &userID},
			{"transfer", types.TxTypeTransfer, &userID, &otherUserID},
			{"withdraw", types.TxTypeWithdraw, &userID, nil},
			{"fee", types.TxTypeFee, &userID, nil},
		}

		for _, tt := range txTypes {
			t.Run(tt.name, func(t *testing.T) {
				now := time.Now().UTC()
				tx := types.Transaction{
					ID:             uuid.Must(uuid.NewV7()),
					IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
					TxType:         tt.txType,
					SenderID:       tt.sender,
					ReceiverID:     tt.receiver,
					AmountSen:      10000,
					Currency:       types.CurrencyIDR,
					Status:         types.TxStatusCommitted,
					CreatedAt:      now,
					CommittedAt:    &now,
				}
				err := store.Append(ctx, tx)
				if err != nil {
					t.Fatalf("Append %s: %v", tt.name, err)
				}
			})
		}
	})
}

func TestConcurrent_TxLogAppend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer cleanup()

	store := NewPostgresTxLogStore(pool)
	userID := createTestUser(ctx, t, pool)
	otherID := createTestUser(ctx, t, pool)

	// Concurrent appends with unique keys should all succeed.
	var wg sync.WaitGroup
	const goroutines = 10
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			now := time.Now().UTC()
			tx := types.Transaction{
				ID:             uuid.Must(uuid.NewV7()),
				IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
				TxType:         types.TxTypeTransfer,
				SenderID:       &userID,
				ReceiverID:     &otherID,
				AmountSen:      int64(1000 * (n + 1)),
				Currency:       types.CurrencyIDR,
				Status:         types.TxStatusCommitted,
				CreatedAt:      now,
				CommittedAt:    &now,
			}
			if err := store.Append(ctx, tx); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent append error: %v", err)
	}

	// Verify all 10 entries exist.
	results, _, err := store.QueryByUserID(ctx, userID, "", 100)
	if err != nil {
		t.Fatalf("QueryByUserID: %v", err)
	}
	if len(results) < goroutines {
		t.Errorf("expected at least %d entries, got %d", goroutines, len(results))
	}
}

// createTestUser inserts a test user and returns their ID.
func createTestUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, phone, pin_hash, kyc_level, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, id.String()+"@phone", "$2a$12$testhash", types.KYCLevelBasic, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}
