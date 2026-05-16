// Package transfer implements the transfer flow: HTTP handler, orchestration service,
// saga coordinator integration, and idempotency guard with Redis.
//
// FCIS structure:
//   - service.go: Shell — orchestrates core + stores, handles serializable tx
//   - handler.go: Shell — pure HTTP concerns (parse, validate, respond)
package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"senpay/internal/fee"
	"senpay/internal/idempotency"
	"senpay/internal/ledger"
	"senpay/internal/nats"
	"senpay/internal/saga"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransferRequest represents a transfer API request body.
type TransferRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	ToPhone        string `json:"to_phone"`
	AmountSen      int64  `json:"amount_sen"`
	Category       string `json:"category,omitempty"`
}

// TransferResult represents a successful transfer response.
type TransferResult struct {
	TxID               uuid.UUID `json:"tx_id"`
	Status             string    `json:"status"`
	AmountSen          int64     `json:"amount_sen"`
	FeeSen             int64     `json:"fee_sen,omitempty"`
	SenderBalanceSen   int64     `json:"sender_balance_sen"`
	ReceiverBalanceSen int64     `json:"receiver_balance_sen"`
	SenderID           uuid.UUID `json:"sender_id"`
	ReceiverID         uuid.UUID `json:"receiver_id"`
	CreatedAt          time.Time `json:"created_at"`
	Cached             bool      `json:"-"` // true if result from idempotency cache (returns 200)
}

// UserStore defines the interface for looking up users.
type UserStore interface {
	FindByPhone(ctx context.Context, phone string) (types.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (types.User, error)
}

// Service handles transfer orchestration with idempotency, saga retry, and compensation.
type Service struct {
	pool            *pgxpool.Pool
	redisCache      *idempotency.RedisIdempotencyCache
	natsClient      *nats.Client
	userStore       UserStore
	sagaCoordinator *saga.SagaCoordinator
	feeConfig       fee.FeeConfig
}

// NewService creates a new transfer Service with the given FeeConfig.
// The feeConfig is used for all fee calculations during transfers.
func NewService(
	pool *pgxpool.Pool,
	redisCache *idempotency.RedisIdempotencyCache,
	natsClient *nats.Client,
	userStore UserStore,
	feeConfig fee.FeeConfig,
) *Service {
	return &Service{
		pool:            pool,
		redisCache:      redisCache,
		natsClient:      natsClient,
		userStore:       userStore,
		sagaCoordinator: saga.NewSagaCoordinator(),
		feeConfig:       feeConfig,
	}
}

// cachedResultSuffix is appended to the idempotency key for storing cached response data.
// The base key stores the status ("completed"), and the suffixed key stores the response JSON.
const cachedResultSuffix = ":cached"

// cachedResult is the data cached in Redis for idempotent responses.
type cachedResult struct {
	Result *TransferResult `json:"result"`
}

// Transfer executes a transfer with idempotency check, saga retry, and compensation.
//
// Flow:
//  1. Validate request fields
//  2. Idempotency check via Redis (proceed / duplicate / in-flight)
//  3. Acquire in-flight marker in Redis (SETNX)
//  4. Execute transfer within SERIALIZABLE transaction with saga retry
//  5. On success: cache result in Redis (24h TTL), publish NATS event
//  6. On failure: clear in-flight marker, compensate, return error
func (s *Service) Transfer(ctx context.Context, senderID uuid.UUID, req TransferRequest) (*TransferResult, *types.DomainError) {
	// 1. Validate request fields.
	if req.IdempotencyKey == "" {
		err := types.NewMissingFieldError("idempotency_key")
		return nil, &err
	}
	if req.ToPhone == "" {
		err := types.NewMissingFieldError("to_phone")
		return nil, &err
	}
	if req.AmountSen <= 0 {
		return nil, &types.ErrInvalidAmount
	}
	if req.AmountSen < types.MinTransferSen {
		return nil, &types.ErrAmountBelowMinimum
	}

	// 2. Idempotency check via Redis.
	status, err := s.redisCache.Get(ctx, req.IdempotencyKey)
	if err != nil {
		slog.Error("redis idempotency check failed", "error", err, "key", req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	decision := idempotency.Check(req.IdempotencyKey, status)
	switch decision {
	case idempotency.Duplicate:
		// Return cached result from Redis.
		return s.getCachedResult(ctx, req.IdempotencyKey)
	case idempotency.InFlight:
		return nil, &types.ErrRequestInFlight
	case idempotency.Proceed:
		// Try to acquire in-flight marker atomically.
		ok, err := s.redisCache.SetIfNotExist(ctx, req.IdempotencyKey, "in_flight", idempotency.InFlightTTL)
		if err != nil {
			slog.Error("redis setnx failed", "error", err, "key", req.IdempotencyKey)
			return nil, &types.ErrInternal
		}
		if !ok {
			// Already acquired by a concurrent request.
			return nil, &types.ErrRequestInFlight
		}
	}

	// 3. Execute transfer with saga retry inside SERIALIZABLE transaction.
	var result *TransferResult

	err = s.sagaCoordinator.Execute(ctx,
		// Operation: execute transfer within SERIALIZABLE transaction.
		func(ctx context.Context) error {
			res, execErr := s.executeTransfer(ctx, senderID, req)
			if execErr != nil {
				slog.Error("transfer operation failed", "error", execErr)
				return execErr
			}
			result = res
			return nil
		},
		// Compensation: clear in-flight marker on exhaustion.
		func(ctx context.Context, originalErr error) {
			slog.Warn("saga retries exhausted, compensating",
				"key", req.IdempotencyKey, "error", originalErr)
			s.compensate(ctx, req.IdempotencyKey)
		},
	)

	if err != nil {
		slog.Error("saga execute returned error", "error", err,
			"key", req.IdempotencyKey)
		if domainErr, ok := saga.AsDomainError(err); ok {
			// Permanent domain error — clear in-flight marker.
			if delErr := s.redisCache.Delete(ctx, req.IdempotencyKey); delErr != nil {
				slog.Warn("failed to clear in-flight marker", "key", req.IdempotencyKey, "error", delErr)
			}
			if delErr := s.redisCache.Delete(ctx, req.IdempotencyKey+cachedResultSuffix); delErr != nil {
				slog.Warn("failed to clear cached result", "key", req.IdempotencyKey, "error", delErr)
			}
			return nil, domainErr
		}
		// Transient or saga exhaustion — already compensated.
		return nil, &types.ErrSerializationConflict
	}

	// 4. Cache success result in Redis (24h TTL).
	s.cacheResult(ctx, req.IdempotencyKey, result)

	// 5. Publish NATS event for tx.completed subject.
	s.publishNatsEvent(ctx, result)

	return result, nil
}

// executeTransfer runs the transfer logic within a PostgreSQL SERIALIZABLE transaction.
func (s *Service) executeTransfer(ctx context.Context, senderID uuid.UUID, req TransferRequest) (*TransferResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Warn("tx rollback failed", "error", err)
		}
	}()

	// Set SERIALIZABLE isolation level.
	_, err = tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE")
	if err != nil {
		slog.Error("failed to set SERIALIZABLE", "error", err)
		return nil, fmt.Errorf("set isolation level: %w", err)
	}

	// Look up receiver by phone.
	receiver, err := s.userStore.FindByPhone(ctx, req.ToPhone)
	if err != nil {
		if isDomainError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("find receiver: %w", err)
	}

	// Check self-transfer.
	if receiver.ID == senderID {
		return nil, types.ErrSelfTransfer
	}

	// Get sender's current balance within transaction (row-level lock via FOR UPDATE).
	senderBalance, err := s.getBalanceForUpdate(ctx, tx, senderID)
	if err != nil {
		return nil, fmt.Errorf("get sender balance: %w", err)
	}

	// Get receiver's current balance within transaction.
	receiverBalance, err := s.getBalanceForUpdate(ctx, tx, receiver.ID)
	if err != nil {
		return nil, fmt.Errorf("get receiver balance: %w", err)
	}

	amount := types.Money(req.AmountSen)

	// Pure core check: can the transfer happen?
	if _, coreErr := ledger.ExecuteTransfer(
		types.Money(senderBalance.BalanceSen),
		types.Money(receiverBalance.BalanceSen),
		amount,
	); coreErr != nil {
		return nil, coreErr
	}

	// Look up sender for KYC level (fee calculation).
	sender, err := s.userStore.FindByID(ctx, senderID)
	if err != nil {
		slog.Error("failed to find sender for fee", "sender_id", senderID, "error", err)
		return nil, fmt.Errorf("find sender: %w", err)
	}

	// Calculate fee based on sender's KYC level and fee config.
	feeAmount, feeErr := fee.CalcFee(amount, sender.KYCLevel, s.feeConfig)
	if feeErr != nil {
		return nil, feeErr
	}

	// Total debit for sender = transfer amount + fee.
	totalDebit := int64(amount) + int64(feeAmount)
	if senderBalance.BalanceSen < totalDebit {
		return nil, &types.ErrInsufficientBalance
	}

	// Update sender balance (debit amount + fee).
	newSenderBalance := senderBalance.BalanceSen - totalDebit
	if err := s.updateBalanceInTx(ctx, tx, senderID, newSenderBalance, senderBalance.Version); err != nil {
		return nil, fmt.Errorf("update sender balance: %w", err)
	}

	// Update receiver balance (credit amount).
	newReceiverBalance := receiverBalance.BalanceSen + int64(amount)
	if err := s.updateBalanceInTx(ctx, tx, receiver.ID, newReceiverBalance, receiverBalance.Version); err != nil {
		return nil, fmt.Errorf("update receiver balance: %w", err)
	}

	now := time.Now().UTC()

	// Insert single transfer tx_log entry with both sender and receiver IDs.
	// A single entry represents the transfer from both perspectives:
	//   - The sender sees this as a debit (sender_id matches → balance decreases)
	//   - The receiver sees this as a credit (receiver_id matches → balance increases)
	// This dual-perspective approach avoids double-counting in balance projection
	// while preserving full counterparty information for both parties.
	txEntry := types.Transaction{
		ID:             uuid.Must(uuid.NewV7()),
		IdempotencyKey: req.IdempotencyKey,
		TxType:         types.TxTypeTransfer,
		SenderID:       &senderID,
		ReceiverID:     &receiver.ID,
		AmountSen:      int64(amount),
		Currency:       types.CurrencyIDR,
		Status:         types.TxStatusCommitted,
		Category:       req.Category,
		CreatedAt:      now,
		CommittedAt:    &now,
	}
	if err := s.appendTxInTx(ctx, tx, txEntry); err != nil {
		return nil, fmt.Errorf("insert transfer tx: %w", err)
	}

	// Insert fee tx_log entry (if fee > 0).
	if int64(feeAmount) > 0 {
		feeTx := types.Transaction{
			ID:             uuid.Must(uuid.NewV7()),
			IdempotencyKey: req.IdempotencyKey,
			TxType:         types.TxTypeFee,
			SenderID:       &senderID,
			AmountSen:      int64(feeAmount),
			Currency:       types.CurrencyIDR,
			Status:         types.TxStatusCommitted,
			CreatedAt:      now,
			CommittedAt:    &now,
		}
		if err := s.appendTxInTx(ctx, tx, feeTx); err != nil {
			return nil, fmt.Errorf("insert fee tx: %w", err)
		}
	}

	// Commit SERIALIZABLE transaction.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &TransferResult{
		TxID:               txEntry.ID,
		Status:             types.TxStatusCommitted.String(),
		AmountSen:          int64(amount),
		FeeSen:             int64(feeAmount),
		SenderBalanceSen:   newSenderBalance,
		ReceiverBalanceSen: newReceiverBalance,
		SenderID:           senderID,
		ReceiverID:         receiver.ID,
		CreatedAt:          now,
	}, nil
}

// isDomainError checks if an error is a DomainError (value or pointer).
func isDomainError(err error) bool {
	_, ok := saga.AsDomainError(err)
	return ok
}

// getBalanceForUpdate reads a user's balance snapshot within a transaction, locking the row.
func (s *Service) getBalanceForUpdate(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (types.BalanceSnapshot, error) {
	const query = `
		SELECT user_id, balance_sen, version, updated_at
		FROM balance_snapshot
		WHERE user_id = $1
		FOR UPDATE
	`
	var snap types.BalanceSnapshot
	err := tx.QueryRow(ctx, query, userID).Scan(
		&snap.UserID, &snap.BalanceSen, &snap.Version, &snap.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No balance row yet — create initial snapshot.
			snap = types.NewBalanceSnapshot(userID)
			if err := s.insertBalanceInTx(ctx, tx, snap); err != nil {
				return types.BalanceSnapshot{}, fmt.Errorf("insert initial balance: %w", err)
			}
			return snap, nil
		}
		return types.BalanceSnapshot{}, fmt.Errorf("query balance: %w", err)
	}
	return snap, nil
}

// insertBalanceInTx inserts a new balance snapshot row within a transaction.
func (s *Service) insertBalanceInTx(ctx context.Context, tx pgx.Tx, snap types.BalanceSnapshot) error {
	const query = `
		INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := tx.Exec(ctx, query, snap.UserID, snap.BalanceSen, snap.Version, snap.UpdatedAt)
	return err
}

// updateBalanceInTx updates a user's balance snapshot with optimistic locking within a transaction.
func (s *Service) updateBalanceInTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, balanceSen int64, version int) error {
	const query = `
		UPDATE balance_snapshot
		SET balance_sen = $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2 AND version = $3
	`
	result, err := tx.Exec(ctx, query, balanceSen, userID, version)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("optimistic lock failure for user %s, expected version %d", userID, version)
	}
	return nil
}

// appendTxInTx inserts a transaction log entry within a transaction.
func (s *Service) appendTxInTx(ctx context.Context, tx pgx.Tx, entry types.Transaction) error {
	const query = `
		INSERT INTO tx_log (
			id, idempotency_key, tx_type, sender_id, receiver_id,
			amount_sen, currency, status, failure_reason, category, created_at, committed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	cat := entry.Category
	if cat == "" {
		cat = types.CategoryDefault
	}
	_, err := tx.Exec(ctx, query,
		entry.ID, entry.IdempotencyKey, entry.TxType,
		entry.SenderID, entry.ReceiverID,
		entry.AmountSen, entry.Currency, entry.Status,
		entry.FailureReason, cat, entry.CreatedAt, entry.CommittedAt,
	)
	if err != nil {
		return fmt.Errorf("insert tx_log: %w", err)
	}
	return nil
}

// getCachedResult retrieves a cached transfer result from Redis for duplicate idempotency keys.
// Reads from the suffixed key (":cached") where the full result JSON is stored.
func (s *Service) getCachedResult(ctx context.Context, key string) (*TransferResult, *types.DomainError) {
	cacheKey := key + cachedResultSuffix
	data, err := s.redisCache.Get(ctx, cacheKey)
	if err != nil || data == "" {
		slog.Warn("expected cached result but not found in Redis", "key", cacheKey)
		return nil, &types.ErrInternal
	}

	var cached cachedResult
	if err := json.Unmarshal([]byte(data), &cached); err != nil || cached.Result == nil {
		slog.Warn("invalid cached result format", "key", key, "error", err)
		return nil, &types.ErrInternal
	}

	// Mark as cached so handler returns 200 instead of 201.
	cached.Result.Cached = true
	return cached.Result, nil
}

// cacheResult stores a successful transfer result in Redis with 24h TTL.
// Uses two keys:
//   - base key: stores "completed" status for idempotency.Check to recognize
//   - suffixed key (":cached"): stores the full result JSON for cached response
func (s *Service) cacheResult(ctx context.Context, key string, result *TransferResult) {
	// Store status as "completed" so idempotency.Check returns Duplicate.
	if err := s.redisCache.Set(ctx, key, "completed", idempotency.DefaultIdempotencyTTL); err != nil {
		slog.Error("failed to cache status in Redis", "key", key, "error", err)
	}

	// Store the full result under a suffixed key.
	cached := cachedResult{Result: result}
	data, err := json.Marshal(cached)
	if err != nil {
		slog.Error("failed to marshal cached result", "key", key, "error", err)
		return
	}

	cacheKey := key + cachedResultSuffix
	if err := s.redisCache.Set(ctx, cacheKey, string(data), idempotency.DefaultIdempotencyTTL); err != nil {
		slog.Error("failed to cache result data in Redis", "key", cacheKey, "error", err)
	}
}

// compensate clears the in-flight marker and any cached data from Redis after saga retry exhaustion.
func (s *Service) compensate(ctx context.Context, idempotencyKey string) {
	if err := s.redisCache.Delete(ctx, idempotencyKey); err != nil {
		slog.Error("failed to clear in-flight marker during compensation",
			"key", idempotencyKey, "error", err)
	}
	// Also clean up any cached result data.
	if err := s.redisCache.Delete(ctx, idempotencyKey+cachedResultSuffix); err != nil {
		slog.Warn("failed to clear cached result during compensation",
			"key", idempotencyKey, "error", err)
	}
}

// natsEventPayload is the event published to NATS on successful transfer.
type natsEventPayload struct {
	TxID       uuid.UUID `json:"tx_id"`
	SenderID   uuid.UUID `json:"sender_id"`
	ReceiverID uuid.UUID `json:"receiver_id"`
	AmountSen  int64     `json:"amount_sen"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// publishNatsEvent publishes a transfer completion event to the tx.completed subject.
func (s *Service) publishNatsEvent(ctx context.Context, result *TransferResult) {
	if s.natsClient == nil || !s.natsClient.IsConnected() {
		slog.Warn("NATS not connected, skipping event publish")
		return
	}

	payload := natsEventPayload{
		TxID:       result.TxID,
		SenderID:   result.SenderID,
		ReceiverID: result.ReceiverID,
		AmountSen:  result.AmountSen,
		Status:     types.TxStatusCommitted.String(),
		CreatedAt:  result.CreatedAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal NATS event", "error", err)
		return
	}

	if err := s.natsClient.Publish("tx.completed", data); err != nil {
		slog.Error("failed to publish NATS event", "error", err)
		return
	}

	slog.Info("published transfer event to NATS",
		"tx_id", result.TxID, "subject", "tx.completed")
}
