//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"senpay/internal/store/storetest"
	"senpay/internal/types"

	"github.com/google/uuid"
)

func TestPostgres_UserStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer cleanup()

	store := NewPostgresUserStore(pool)

	t.Run("Insert_and_FindByID", func(t *testing.T) {
		id := uuid.Must(uuid.NewV7())
		user := types.User{
			ID:        id,
			Phone:     "081234567890",
			PINHash:   "$2a$12$testhash",
			KYCLevel:  types.KYCLevelBasic,
			CreatedAt: time.Now().UTC(),
		}

		err := store.Insert(ctx, user)
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		got, err := store.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.ID != user.ID {
			t.Errorf("ID: got %v, want %v", got.ID, user.ID)
		}
		if got.Phone != user.Phone {
			t.Errorf("Phone: got %s, want %s", got.Phone, user.Phone)
		}
		if got.KYCLevel != user.KYCLevel {
			t.Errorf("KYCLevel: got %s, want %s", got.KYCLevel, user.KYCLevel)
		}
	})

	t.Run("FindByPhone", func(t *testing.T) {
		id := uuid.Must(uuid.NewV7())
		user := types.User{
			ID:        id,
			Phone:     "081111111111",
			PINHash:   "$2a$12$testhash",
			KYCLevel:  types.KYCLevelBasic,
			CreatedAt: time.Now().UTC(),
		}

		err := store.Insert(ctx, user)
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		got, err := store.FindByPhone(ctx, "081111111111")
		if err != nil {
			t.Fatalf("FindByPhone: %v", err)
		}
		if got.ID != id {
			t.Errorf("ID: got %v, want %v", got.ID, id)
		}
	})

	t.Run("FindByPhone_NotFound", func(t *testing.T) {
		_, err := store.FindByPhone(ctx, "089999999999")
		if err == nil {
			t.Fatal("expected error for nonexistent phone")
		}
		if domainErr, ok := err.(types.DomainError); ok {
			if domainErr.Code != types.ErrCodeUserNotFound {
				t.Errorf("expected USER_NOT_FOUND, got %s", domainErr.Code)
			}
		} else {
			t.Logf("got error type: %T, value: %v", err, err)
		}
	})

	t.Run("UpdateKYCLevel", func(t *testing.T) {
		id := uuid.Must(uuid.NewV7())
		user := types.User{
			ID:        id,
			Phone:     "082222222222",
			PINHash:   "$2a$12$testhash",
			KYCLevel:  types.KYCLevelBasic,
			CreatedAt: time.Now().UTC(),
		}

		err := store.Insert(ctx, user)
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		err = store.UpdateKYCLevel(ctx, id, types.KYCLevelVerified)
		if err != nil {
			t.Fatalf("UpdateKYCLevel: %v", err)
		}

		got, err := store.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.KYCLevel != types.KYCLevelVerified {
			t.Errorf("KYCLevel: got %s, want %s", got.KYCLevel, types.KYCLevelVerified)
		}
	})

	t.Run("UpdateKYCLevel_UserNotFound", func(t *testing.T) {
		err := store.UpdateKYCLevel(ctx, uuid.Must(uuid.NewV7()), types.KYCLevelVerified)
		if err == nil {
			t.Fatal("expected error for nonexistent user")
		}
	})

	t.Run("Insert_DuplicatePhone", func(t *testing.T) {
		phone := "083333333333"
		id1 := uuid.Must(uuid.NewV7())
		user1 := types.User{
			ID:        id1,
			Phone:     phone,
			PINHash:   "$2a$12$hash1",
			KYCLevel:  types.KYCLevelBasic,
			CreatedAt: time.Now().UTC(),
		}
		err := store.Insert(ctx, user1)
		if err != nil {
			t.Fatalf("Insert first user: %v", err)
		}

		id2 := uuid.Must(uuid.NewV7())
		user2 := types.User{
			ID:        id2,
			Phone:     phone,
			PINHash:   "$2a$12$hash2",
			KYCLevel:  types.KYCLevelBasic,
			CreatedAt: time.Now().UTC(),
		}
		err = store.Insert(ctx, user2)
		if err == nil {
			t.Fatal("expected error for duplicate phone")
		}
	})
}
