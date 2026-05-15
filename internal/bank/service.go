package bank

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"senpay/internal/idempotency"
	"senpay/internal/nats"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ────────────────────────────────────────────────────────────────
// Service Constants
// ────────────────────────────────────────────────────────────────

const (
	// cachedResultSuffix is appended to the idempotency key for storing cached response data.
	topupCachedResultSuffix = ":cached"
)

// ────────────────────────────────────────────────────────────────
// Top-up Service
// ────────────────────────────────────────────────────────────────

// TopupRequest is the HTTP-level top-up request.
type TopupHTTPRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	AmountSen      int64  `json:"amount_sen"`
}

// TopupHTTPResponse is the HTTP-level top-up response.
type TopupHTTPResponse struct {
	TxID      uuid.UUID `json:"tx_id"`
	VANumber  string    `json:"va_number"`
	AmountSen int64     `json:"amount_sen"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// UserStore defines the interface for looking up users (subset of auth.UserRepository).
type UserStore interface {
	FindByID(ctx context.Context, id uuid.UUID) (types.User, error)
}

// Service orchestrates bank operations: top-up, webhook processing, withdraw.
type Service struct {
	pool           *pgxpool.Pool
	vaStore        VAStore
	withdrawStore  WithdrawStore
	redisCache     *idempotency.RedisIdempotencyCache
	paymentRail    PaymentRail
	natsClient     *nats.Client
	userStore      UserStore
}

// NewService creates a new bank Service.
func NewService(
	pool *pgxpool.Pool,
	vaStore VAStore,
	withdrawStore WithdrawStore,
	redisCache *idempotency.RedisIdempotencyCache,
	paymentRail PaymentRail,
	natsClient *nats.Client,
	userStore UserStore,
) *Service {
	return &Service{
		pool:          pool,
		vaStore:       vaStore,
		withdrawStore: withdrawStore,
		redisCache:    redisCache,
		paymentRail:   paymentRail,
		natsClient:    natsClient,
		userStore:     userStore,
	}
}

// ── Cached Result ──────────────────────────────────────────────

// topupCachedResult is the data cached in Redis for idempotent responses.
type topupCachedResult struct {
	Result *TopupHTTPResponse `json:"result"`
}

// ── Top-up Flow ────────────────────────────────────────────────

// Topup initiates a top-up request for the given user.
//
// Flow:
//  1. Validate request fields (amount > 0, idempotency_key present)
//  2. Check idempotency (Redis)
//  3. Acquire in-flight marker
//  4. Look up user's KYC level for BI limit check
//  5. Generate VA number (pure core function)
//  6. Insert VA record + pending tx_log in PG transaction
//  7. Send credit request to bank (via PaymentRail adapter)
//  8. Return VA details to caller
func (s *Service) Topup(ctx context.Context, userID uuid.UUID, req TopupHTTPRequest) (*TopupHTTPResponse, *types.DomainError) {
	// 1. Validate request fields.
	if req.IdempotencyKey == "" {
		err := types.NewMissingFieldError("idempotency_key")
		return nil, &err
	}
	if req.AmountSen <= 0 {
		return nil, &types.ErrInvalidAmount
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
			return nil, &types.ErrRequestInFlight
		}
	}

	// 3. Look up user for KYC level (BI limit check).
	user, err := s.userStore.FindByID(ctx, userID)
	if err != nil {
		slog.Error("failed to find user", "user_id", userID, "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// 4. BI limit check.
	if limitErr := checkBILimit(types.Money(req.AmountSen), user.KYCLevel); limitErr != nil {
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, limitErr
	}

	// 5. Pure core: generate VA number and top-up details.
	coreReq := TopupRequest{
		UserID:         userID,
		IdempotencyKey: req.IdempotencyKey,
		AmountSen:      req.AmountSen,
	}
	coreResult, coreErr := GenerateTopupCore(coreReq)
	if coreErr != nil {
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, coreErr
	}

	// 6. Execute within a PostgreSQL transaction:
	//    a. Insert VA record
	//    b. Create pending tx_log entry
	var response *TopupHTTPResponse

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		slog.Error("failed to begin tx", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Warn("tx rollback failed", "error", err)
		}
	}()

	// a. Create pending tx_log entry (receiver = user, sender = nil for top-up).
	now := time.Now().UTC()
	txLogEntry := types.Transaction{
		ID:             uuid.Must(uuid.NewV7()),
		IdempotencyKey: req.IdempotencyKey,
		TxType:         types.TxTypeTopup,
		SenderID:       nil, // no sender for top-up
		ReceiverID:     &userID,
		AmountSen:      req.AmountSen,
		Currency:       types.CurrencyIDR,
		Status:         types.TxStatusPending,
		CreatedAt:      now,
		CommittedAt:    nil,
	}

	if err := s.appendTxInTx(ctx, tx, txLogEntry); err != nil {
		slog.Error("failed to insert tx_log", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// b. Insert VA record linked to the tx_log entry.
	vaRecord := VATopupRecord{
		ID:             coreResult.ID,
		IdempotencyKey: req.IdempotencyKey,
		UserID:         userID,
		VANumber:       coreResult.VANumber,
		AmountSen:      req.AmountSen,
		Status:         "active",
		CreatedAt:      now,
		ExpiresAt:      coreResult.ExpiresAt,
		PaidAt:         nil,
		TxLogID:        &txLogEntry.ID,
	}

	if err := s.insertVAInTx(ctx, tx, vaRecord); err != nil {
		slog.Error("failed to insert va_topup", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// Commit transaction.
	if err := tx.Commit(ctx); err != nil {
		slog.Error("failed to commit tx", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// 7. Build response.
	response = &TopupHTTPResponse{
		TxID:      txLogEntry.ID,
		VANumber:  coreResult.VANumber,
		AmountSen: req.AmountSen,
		Status:    types.TxStatusPending,
		ExpiresAt: coreResult.ExpiresAt,
		CreatedAt: now,
	}

	// 8. Send credit request to bank adapter (SNAP or stub).
	//    This triggers the mock bank to process the VA payment and send
	//    a webhook callback. If the bank rejects, the VA remains pending
	//    (the webhook will not fire).
	creditReq := CreditRequest{
		VANumber:      coreResult.VANumber,
		AmountSen:     req.AmountSen,
		PartnerID:     SNAPPartnerID,
		ExternalID:    req.IdempotencyKey,
		TransactionID: txLogEntry.ID,
		Timestamp:     now,
	}

	if creditResult, creditErr := s.paymentRail.Credit(ctx, creditReq); creditErr != nil {
		slog.Warn("bank credit request failed (VA still created)",
			"va_number", coreResult.VANumber,
			"error", creditErr.Code,
		)
	} else if creditResult != nil {
		slog.Info("bank credit request sent successfully",
			"va_number", coreResult.VANumber,
			"reference_id", creditResult.ReferenceID,
		)
	}

	// 9. Update idempotency status (completed) and cache the result.
	s.cacheResult(ctx, req.IdempotencyKey, response)

	return response, nil
}

// ── Withdraw Flow ─────────────────────────────────────────────

const (
	// withdrawTimeout is the maximum time to wait for a bank withdraw response.
	withdrawTimeout = 30 * time.Second
	// withdrawCachedResultSuffix is appended to the idempotency key for storing cached withdraw response data.
	withdrawCachedResultSuffix = ":wd:cached"
)

// WithdrawHTTPRequest is the HTTP-level withdraw request.
type WithdrawHTTPRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	AmountSen      int64  `json:"amount_sen"`
	BankAccount    string `json:"bank_account"`
}

// WithdrawHTTPResponse is the HTTP-level withdraw response.
type WithdrawHTTPResponse struct {
	TxID        uuid.UUID `json:"tx_id"`
	AmountSen   int64     `json:"amount_sen"`
	BankAccount string    `json:"bank_account"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// withdrawCachedResult is the data cached in Redis for idempotent withdraw responses.
type withdrawCachedResult struct {
	Result *WithdrawHTTPResponse `json:"result"`
}

// getWithdrawCachedResult retrieves a cached withdraw result from Redis.
func (s *Service) getWithdrawCachedResult(ctx context.Context, key string) (*WithdrawHTTPResponse, *types.DomainError) {
	cacheKey := key + withdrawCachedResultSuffix
	data, err := s.redisCache.Get(ctx, cacheKey)
	if err != nil || data == "" {
		slog.Warn("expected cached withdraw result but not found in Redis", "key", cacheKey)
		return nil, &types.ErrInternal
	}

	var cached withdrawCachedResult
	if err := json.Unmarshal([]byte(data), &cached); err != nil || cached.Result == nil {
		slog.Warn("invalid cached withdraw result format", "key", key, "error", err)
		return nil, &types.ErrInternal
	}

	return cached.Result, nil
}

// cacheWithdrawResult stores a successful withdraw result in Redis with 24h TTL.
func (s *Service) cacheWithdrawResult(ctx context.Context, key string, result *WithdrawHTTPResponse) {
	if err := s.redisCache.Set(ctx, key, "completed", idempotency.DefaultIdempotencyTTL); err != nil {
		slog.Error("failed to cache withdraw status in Redis", "key", key, "error", err)
	}

	cached := withdrawCachedResult{Result: result}
	data, err := json.Marshal(cached)
	if err != nil {
		slog.Error("failed to marshal cached withdraw result", "key", key, "error", err)
		return
	}

	cacheKey := key + withdrawCachedResultSuffix
	if err := s.redisCache.Set(ctx, cacheKey, string(data), idempotency.DefaultIdempotencyTTL); err != nil {
		slog.Error("failed to cache withdraw result data in Redis", "key", cacheKey, "error", err)
	}
}

// Withdraw initiates a withdraw request for the given user.
//
// Flow:
//  1. Validate request fields (amount > 0, idempotency_key present, bank_account non-empty)
//  2. Check idempotency (Redis)
//  3. Acquire in-flight marker
//  4. Look up user's KYC level for BI limit check
//  5. Check user's balance (sufficient funds)
//  6. In a PG transaction: reserve funds (debit balance), create pending tx_log, create withdraw_record
//  7. Send withdraw request to bank (via PaymentRail adapter) with 30s timeout context
//  8. On success: mark as committed, cache result
//  9. On rejection: reverse funds, mark as failed, return Indonesian error
//  10. On timeout: reverse funds, mark as timeout, return timeout error
func (s *Service) Withdraw(ctx context.Context, userID uuid.UUID, req WithdrawHTTPRequest) (*WithdrawHTTPResponse, *types.DomainError) {
	// 1. Validate request fields.
	if req.IdempotencyKey == "" {
		err := types.NewMissingFieldError("idempotency_key")
		return nil, &err
	}
	if req.AmountSen <= 0 {
		return nil, &types.ErrInvalidAmount
	}
	if req.BankAccount == "" {
		err := types.NewMissingFieldError("bank_account")
		return nil, &err
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
		return s.getWithdrawCachedResult(ctx, req.IdempotencyKey)
	case idempotency.InFlight:
		return nil, &types.ErrRequestInFlight
	case idempotency.Proceed:
		ok, err := s.redisCache.SetIfNotExist(ctx, req.IdempotencyKey, "in_flight", idempotency.InFlightTTL)
		if err != nil {
			slog.Error("redis setnx failed", "error", err, "key", req.IdempotencyKey)
			return nil, &types.ErrInternal
		}
		if !ok {
			return nil, &types.ErrRequestInFlight
		}
	}

	// 3. Look up user for KYC level (BI limit check).
	user, err := s.userStore.FindByID(ctx, userID)
	if err != nil {
		slog.Error("failed to find user", "user_id", userID, "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// 4. BI limit check.
	if limitErr := checkBILimit(types.Money(req.AmountSen), user.KYCLevel); limitErr != nil {
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, limitErr
	}

	// 5. Execute reserve step within a PostgreSQL transaction:
	//    a. Lock and check balance (sufficient funds)
	//    b. Debit balance (reserve)
	//    c. Create pending tx_log entry
	//    d. Create withdraw_record
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		slog.Error("failed to begin tx", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	var rollback = true
	defer func() {
		if rollback {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				slog.Warn("tx rollback failed", "error", err)
			}
		}
	}()

	now := time.Now().UTC()

	// a. Lock and check balance.
	balanceSen, version, err := s.getBalanceForUpdate(ctx, tx, userID)
	if err != nil {
		slog.Error("failed to get balance", "user_id", userID, "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	if balanceSen < req.AmountSen {
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInsufficientBalance
	}

	// b. Debit balance (reserve).
	newBalance := balanceSen - req.AmountSen
	if err := s.updateBalance(ctx, tx, userID, newBalance, version); err != nil {
		slog.Error("failed to update balance", "user_id", userID, "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// c. Create pending tx_log entry (sender = user for withdraw).
	txLogID := uuid.Must(uuid.NewV7())
	txLogEntry := types.Transaction{
		ID:             txLogID,
		IdempotencyKey: req.IdempotencyKey,
		TxType:         types.TxTypeWithdraw,
		SenderID:       &userID,
		ReceiverID:     nil,
		AmountSen:      req.AmountSen,
		Currency:       types.CurrencyIDR,
		Status:         types.TxStatusPending,
		CreatedAt:      now,
		CommittedAt:    nil,
	}

	if err := s.appendTxInTx(ctx, tx, txLogEntry); err != nil {
		slog.Error("failed to insert tx_log", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// d. Create withdraw_record.
	wdID := uuid.Must(uuid.NewV7())
	wdRecord := WithdrawRecord{
		ID:             wdID,
		IdempotencyKey: req.IdempotencyKey,
		UserID:         userID,
		BankAccount:    req.BankAccount,
		AmountSen:      req.AmountSen,
		Status:         types.TxStatusPending,
		CreatedAt:      now,
		TxLogID:        &txLogID,
	}

	if err := s.insertWithdrawInTx(ctx, tx, wdRecord); err != nil {
		slog.Error("failed to insert withdraw_record", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}

	// Commit the reserve transaction.
	if err := tx.Commit(ctx); err != nil {
		slog.Error("failed to commit reserve tx", "error", err)
		_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
		return nil, &types.ErrInternal
	}
	rollback = false

	// Build initial response.
	response := &WithdrawHTTPResponse{
		TxID:        txLogID,
		AmountSen:   req.AmountSen,
		BankAccount: req.BankAccount,
		Status:      types.TxStatusPending,
		CreatedAt:   now,
	}

	// 6. Send withdraw request to bank adapter with timeout context.
	bankCtx, cancel := context.WithTimeout(ctx, withdrawTimeout)
	defer cancel()

	withdrawReq := WithdrawRequest{
		BankAccount:   req.BankAccount,
		AmountSen:     req.AmountSen,
		PartnerID:     SNAPPartnerID,
		ExternalID:    req.IdempotencyKey,
		TransactionID: txLogID,
		Timestamp:     now,
	}

	bankResult, bankErr := s.paymentRail.Withdraw(bankCtx, withdrawReq)

	// 7. Handle bank response.
	if bankErr != nil {
		slog.Warn("bank withdraw request failed",
			"error", bankErr.Code,
			"external_id", req.IdempotencyKey,
			"tx_log_id", txLogID)

		// Reverse: re-credit the user's balance.
		reverseErr := s.reverseWithdraw(ctx, userID, req.AmountSen, txLogID, wdID, bankErr)
		if reverseErr != nil {
			slog.Error("failed to reverse withdraw", "tx_log_id", txLogID, "error", reverseErr)
			// Return original bank error — reversal is best-effort.
		}

		return nil, bankErr
	}

	if bankResult != nil && bankResult.Success {
		// Bank confirmed success — mark as committed.
		if err := s.commitWithdraw(ctx, txLogID, wdID, now); err != nil {
			slog.Error("failed to commit withdraw", "tx_log_id", txLogID, "error", err)
			_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
			return nil, &types.ErrInternal
		}

		slog.Info("withdraw committed successfully",
			"tx_log_id", txLogID,
			"amount_sen", req.AmountSen,
			"reference_id", bankResult.ReferenceID)

		// Publish NATS event.
		s.publishWithdrawEvent(ctx, userID, txLogID, req.AmountSen, now)

		response.Status = types.TxStatusCommitted
		s.cacheWithdrawResult(ctx, req.IdempotencyKey, response)
		return response, nil
	}

	// Unexpected case: bank returned nil result and nil error.
	slog.Error("bank withdraw returned nil result and nil error", "external_id", req.IdempotencyKey)
	_ = s.redisCache.Delete(ctx, req.IdempotencyKey)
	return nil, &types.ErrInternal
}

// reverseWithdraw reverses a failed withdraw: re-credits the user's balance and marks records as failed/timeout.
func (s *Service) reverseWithdraw(ctx context.Context, userID uuid.UUID, amountSen int64, txLogID, wdID uuid.UUID, bankErr *types.DomainError) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reverse tx: %w", err)
	}

	var rollback = true
	defer func() {
		if rollback {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				slog.Warn("reverse tx rollback failed", "error", err)
			}
		}
	}()

	now := time.Now().UTC()

	// Determine status based on bank error type.
	var newStatus string
	var failureReason string
	switch bankErr.Code {
	case ErrTimeout.Code:
		newStatus = types.TxStatusTimeout
		failureReason = "Bank timeout: " + bankErr.Message
	case ErrBankRejection.Code:
		newStatus = types.TxStatusFailed
		failureReason = "Bank rejection: " + bankErr.Message
	default:
		newStatus = types.TxStatusFailed
		failureReason = bankErr.Message
	}

	// Re-credit user's balance.
	if err := s.applyCredit(ctx, tx, userID, amountSen); err != nil {
		return fmt.Errorf("re-credit balance: %w", err)
	}

	// Update tx_log status.
	if err := s.updateTxLogStatus(ctx, tx, txLogID, newStatus, &failureReason, nil); err != nil {
		return fmt.Errorf("update tx_log: %w", err)
	}

	// Update withdraw_record status.
	reversedAt := now
	if err := s.withdrawStore.UpdateStatus(ctx, wdID, newStatus, &failureReason, nil, &reversedAt); err != nil {
		return fmt.Errorf("update withdraw_record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reverse tx: %w", err)
	}
	rollback = false

	slog.Info("withdraw reversed successfully",
		"tx_log_id", txLogID,
		"user_id", userID,
		"amount_sen", amountSen,
		"status", newStatus)

	return nil
}

// commitWithdraw marks a withdraw as committed (updates tx_log and withdraw_record).
func (s *Service) commitWithdraw(ctx context.Context, txLogID, wdID uuid.UUID, committedAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin commit tx: %w", err)
	}

	var rollback = true
	defer func() {
		if rollback {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				slog.Warn("commit tx rollback failed", "error", err)
			}
		}
	}()

	// Update tx_log status to committed.
	if err := s.commitTxLog(ctx, tx, txLogID, committedAt); err != nil {
		return fmt.Errorf("commit tx_log: %w", err)
	}

	// Update withdraw_record status to committed.
	now := time.Now().UTC()
	if err := s.withdrawStore.UpdateStatus(ctx, wdID, types.TxStatusCommitted, nil, &now, nil); err != nil {
		return fmt.Errorf("update withdraw_record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit withdraw tx: %w", err)
	}
	rollback = false

	return nil
}

// insertWithdrawInTx inserts a withdraw record within a transaction.
func (s *Service) insertWithdrawInTx(ctx context.Context, tx pgx.Tx, record WithdrawRecord) error {
	const query = `
		INSERT INTO withdraw_records (
			id, idempotency_key, user_id, bank_account, amount_sen,
			status, failure_reason, created_at, committed_at, reversed_at, tx_log_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := tx.Exec(ctx, query,
		record.ID, record.IdempotencyKey, record.UserID, record.BankAccount,
		record.AmountSen, record.Status, record.FailureReason, record.CreatedAt,
		record.CommittedAt, record.ReversedAt, record.TxLogID,
	)
	if err != nil {
		return fmt.Errorf("insert withdraw_record: %w", err)
	}
	return nil
}

// getBalanceForUpdate retrieves the user's balance with a FOR UPDATE lock.
func (s *Service) getBalanceForUpdate(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (int64, int, error) {
	const query = `
		SELECT balance_sen, version FROM balance_snapshot WHERE user_id = $1 FOR UPDATE
	`
	var balanceSen int64
	var version int
	err := tx.QueryRow(ctx, query, userID).Scan(&balanceSen, &version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No balance row means zero balance.
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("query balance: %w", err)
	}
	return balanceSen, version, nil
}

// updateBalance updates the user's balance with optimistic locking.
func (s *Service) updateBalance(ctx context.Context, tx pgx.Tx, userID uuid.UUID, newBalance int64, version int) error {
	if version == 0 {
		// No existing balance row — insert initial snapshot.
		const insertQuery = `
			INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
			VALUES ($1, $2, 1, NOW())
		`
		_, err := tx.Exec(ctx, insertQuery, userID, newBalance)
		if err != nil {
			return fmt.Errorf("insert initial balance: %w", err)
		}
		return nil
	}

	const updateQuery = `
		UPDATE balance_snapshot
		SET balance_sen = $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2 AND version = $3
	`
	result, err := tx.Exec(ctx, updateQuery, newBalance, userID, version)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("optimistic lock failure for user %s", userID)
	}
	return nil
}

// updateTxLogStatus updates a tx_log entry's status within a transaction.
func (s *Service) updateTxLogStatus(ctx context.Context, tx pgx.Tx, txLogID uuid.UUID, status string, failureReason *string, committedAt *time.Time) error {
	const query = `
		UPDATE tx_log
		SET status = $2, failure_reason = $3, committed_at = $4
		WHERE id = $1
	`
	_, err := tx.Exec(ctx, query, txLogID, status, failureReason, committedAt)
	if err != nil {
		return fmt.Errorf("update tx_log status: %w", err)
	}
	return nil
}

// ── Withdraw Webhook ───────────────────────────────────────────

// ProcessWithdrawCallback processes a bank webhook callback for a withdraw.
// This is called when the bank sends a confirmation callback after a withdraw.
// It is idempotent: if the withdraw is already committed, it returns success.
func (s *Service) ProcessWithdrawCallback(ctx context.Context, callback *BankCallback) *types.DomainError {
	if callback == nil {
		err := types.NewMissingFieldError("webhook body")
		return &err
	}

	// Find withdraw record by external ID (idempotency_key).
	wdRecord, err := s.withdrawStore.FindByIdempotencyKey(ctx, callback.ExternalID)
	if err != nil {
		slog.Error("failed to find withdraw record", "external_id", callback.ExternalID, "error", err)
		return &types.ErrInternal
	}
	if wdRecord == nil {
		slog.Warn("withdraw record not found for webhook", "external_id", callback.ExternalID)
		return &types.ErrUserNotFound
	}

	// Idempotent: if already committed, skip.
	if wdRecord.Status == types.TxStatusCommitted {
		slog.Info("withdraw already committed, skipping webhook", "external_id", callback.ExternalID)
		return nil
	}

	// Only process if still pending.
	if wdRecord.Status != types.TxStatusPending {
		slog.Warn("withdraw not in pending state for webhook",
			"external_id", callback.ExternalID,
			"status", wdRecord.Status)
		return nil
	}

	now := time.Now().UTC()

	// Mark as committed.
	if wdRecord.TxLogID != nil {
		if err := s.commitWithdraw(ctx, *wdRecord.TxLogID, wdRecord.ID, now); err != nil {
			slog.Error("failed to commit withdraw from webhook",
				"tx_log_id", *wdRecord.TxLogID, "error", err)
			return &types.ErrInternal
		}

		slog.Info("withdraw webhook processed successfully",
			"tx_log_id", *wdRecord.TxLogID,
			"user_id", wdRecord.UserID,
			"amount_sen", wdRecord.AmountSen)

		// Publish NATS event.
		s.publishWithdrawEvent(ctx, wdRecord.UserID, *wdRecord.TxLogID, wdRecord.AmountSen, now)
	}

	return nil
}

// ── Withdraw NATS Event ───────────────────────────────────────

// natsWithdrawEvent is the event published to NATS on successful withdraw commit.
type natsWithdrawEvent struct {
	TxID      uuid.UUID `json:"tx_id"`
	UserID    uuid.UUID `json:"user_id"`
	AmountSen int64     `json:"amount_sen"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// publishWithdrawEvent publishes a withdraw completion event to NATS.
func (s *Service) publishWithdrawEvent(ctx context.Context, userID uuid.UUID, txLogID uuid.UUID, amountSen int64, now time.Time) {
	if s.natsClient == nil || !s.natsClient.IsConnected() {
		slog.Warn("NATS not connected, skipping withdraw event publish")
		return
	}

	payload := natsWithdrawEvent{
		TxID:      txLogID,
		UserID:    userID,
		AmountSen: amountSen,
		Status:    types.TxStatusCommitted,
		CreatedAt: now,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal NATS withdraw event", "error", err)
		return
	}

	if err := s.natsClient.Publish("tx.completed", data); err != nil {
		slog.Error("failed to publish NATS withdraw event", "error", err)
		return
	}

	slog.Info("published withdraw event to NATS",
		"tx_id", payload.TxID, "subject", "tx.completed")
}

// ── Webhook Callback Processing ────────────────────────────────

// ProcessWebhook processes a bank webhook callback for a VA payment.
//
// Flow:
//  1. Parse webhook callback
//  2. Find VA record by VA number
//  3. Verify VA is active (not already paid/expired)
//  4. Update tx_log from pending to committed
//  5. Apply credit to user's balance (upsert balance_snapshot)
//  6. Mark VA as paid
//  7. Publish NATS event
func (s *Service) ProcessWebhook(ctx context.Context, callback *BankCallback) *types.DomainError {
	if callback == nil {
		err := types.NewMissingFieldError("webhook body")
		return &err
	}

	// 2. Find VA record by VA number.
	vaRecord, err := s.vaStore.FindByVANumber(ctx, callback.VANumber)
	if err != nil {
		slog.Error("failed to find VA record", "va_number", callback.VANumber, "error", err)
		return &types.ErrInternal
	}
	if vaRecord == nil {
		slog.Warn("VA record not found for webhook", "va_number", callback.VANumber)
		return &ErrInvalidVA
	}

	// 3. Verify VA is still active.
	if vaRecord.Status != "active" {
		slog.Warn("VA already processed", "va_number", callback.VANumber,
			"status", vaRecord.Status)
		return &ErrDuplicateRequest
	}

	// 4. Execute within a PostgreSQL transaction:
	//    a. Update tx_log status to committed
	//    b. Update/insert balance_snapshot
	//    c. Mark VA as paid
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		slog.Error("failed to begin webhook tx", "error", err)
		return &types.ErrInternal
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Warn("webhook tx rollback failed", "error", err)
		}
	}()

	now := time.Now().UTC()

	// a. Update tx_log status to committed.
	if vaRecord.TxLogID != nil {
		if err := s.commitTxLog(ctx, tx, *vaRecord.TxLogID, now); err != nil {
			slog.Error("failed to commit tx_log", "tx_log_id", *vaRecord.TxLogID, "error", err)
			return &types.ErrInternal
		}
	}

	// b. Apply credit to user's balance.
	if err := s.applyCredit(ctx, tx, vaRecord.UserID, vaRecord.AmountSen); err != nil {
		slog.Error("failed to apply credit", "user_id", vaRecord.UserID, "error", err)
		return &types.ErrInternal
	}

	// c. Mark VA as paid.
	if err := s.markVAPaidInTx(ctx, tx, callback.VANumber, *vaRecord.TxLogID); err != nil {
		slog.Error("failed to mark VA as paid", "va_number", callback.VANumber, "error", err)
		return &types.ErrInternal
	}

	// Commit transaction.
	if err := tx.Commit(ctx); err != nil {
		slog.Error("failed to commit webhook tx", "error", err)
		return &types.ErrInternal
	}

	slog.Info("webhook processed successfully",
		"va_number", callback.VANumber,
		"user_id", vaRecord.UserID,
		"amount_sen", vaRecord.AmountSen)

	// 7. Publish NATS event.
	s.publishTopupEvent(ctx, vaRecord, now)

	return nil
}

// ── Transaction Helpers ────────────────────────────────────────

// appendTxInTx inserts a transaction log entry within a transaction.
func (s *Service) appendTxInTx(ctx context.Context, tx pgx.Tx, entry types.Transaction) error {
	const query = `
		INSERT INTO tx_log (
			id, idempotency_key, tx_type, sender_id, receiver_id,
			amount_sen, currency, status, failure_reason, created_at, committed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := tx.Exec(ctx, query,
		entry.ID, entry.IdempotencyKey, entry.TxType,
		entry.SenderID, entry.ReceiverID,
		entry.AmountSen, entry.Currency, entry.Status,
		entry.FailureReason, entry.CreatedAt, entry.CommittedAt,
	)
	if err != nil {
		return fmt.Errorf("insert tx_log: %w", err)
	}
	return nil
}

// insertVAInTx inserts a VA record within a transaction.
func (s *Service) insertVAInTx(ctx context.Context, tx pgx.Tx, record VATopupRecord) error {
	const query = `
		INSERT INTO va_topup (
			id, idempotency_key, user_id, va_number, amount_sen,
			status, created_at, expires_at, paid_at, tx_log_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := tx.Exec(ctx, query,
		record.ID, record.IdempotencyKey, record.UserID, record.VANumber,
		record.AmountSen, record.Status, record.CreatedAt, record.ExpiresAt,
		record.PaidAt, record.TxLogID,
	)
	if err != nil {
		return fmt.Errorf("insert va_topup: %w", err)
	}
	return nil
}

// commitTxLog updates a tx_log entry status to committed within a transaction.
func (s *Service) commitTxLog(ctx context.Context, tx pgx.Tx, txLogID uuid.UUID, committedAt time.Time) error {
	const query = `
		UPDATE tx_log
		SET status = 'committed', committed_at = $2
		WHERE id = $1 AND status = 'pending'
	`
	result, err := tx.Exec(ctx, query, txLogID, committedAt)
	if err != nil {
		return fmt.Errorf("commit tx_log: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("tx_log not found or already committed: %s", txLogID)
	}
	return nil
}

// applyCredit updates a user's balance_snapshot by adding the top-up amount.
func (s *Service) applyCredit(ctx context.Context, tx pgx.Tx, userID uuid.UUID, amountSen int64) error {
	// Get current balance (or create new snapshot).
	const selectQuery = `
		SELECT balance_sen, version FROM balance_snapshot WHERE user_id = $1 FOR UPDATE
	`
	var balanceSen int64
	var version int
	err := tx.QueryRow(ctx, selectQuery, userID).Scan(&balanceSen, &version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No balance row — insert initial snapshot with the credit.
			const insertQuery = `
				INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
				VALUES ($1, $2, 1, NOW())
			`
			_, err := tx.Exec(ctx, insertQuery, userID, amountSen)
			if err != nil {
				return fmt.Errorf("insert initial balance: %w", err)
			}
			return nil
		}
		return fmt.Errorf("query balance: %w", err)
	}

	// Update balance with optimistic lock.
	newBalance := balanceSen + amountSen
	const updateQuery = `
		UPDATE balance_snapshot
		SET balance_sen = $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2 AND version = $3
	`
	result, err := tx.Exec(ctx, updateQuery, newBalance, userID, version)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("optimistic lock failure for user %s", userID)
	}
	return nil
}

// markVAPaidInTx marks a VA record as paid within a transaction.
func (s *Service) markVAPaidInTx(ctx context.Context, tx pgx.Tx, vaNumber string, txLogID uuid.UUID) error {
	const query = `
		UPDATE va_topup
		SET status = 'paid', paid_at = NOW(), tx_log_id = $2
		WHERE va_number = $1 AND status = 'active'
	`
	result, err := tx.Exec(ctx, query, vaNumber, txLogID)
	if err != nil {
		return fmt.Errorf("mark va as paid: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("VA not found or already paid: %s", vaNumber)
	}
	return nil
}

// ── Idempotency Helpers ────────────────────────────────────────

// getCachedResult retrieves a cached top-up result from Redis.
func (s *Service) getCachedResult(ctx context.Context, key string) (*TopupHTTPResponse, *types.DomainError) {
	cacheKey := key + topupCachedResultSuffix
	data, err := s.redisCache.Get(ctx, cacheKey)
	if err != nil || data == "" {
		slog.Warn("expected cached result but not found in Redis", "key", cacheKey)
		return nil, &types.ErrInternal
	}

	var cached topupCachedResult
	if err := json.Unmarshal([]byte(data), &cached); err != nil || cached.Result == nil {
		slog.Warn("invalid cached result format", "key", key, "error", err)
		return nil, &types.ErrInternal
	}

	return cached.Result, nil
}

// cacheResult stores a successful top-up result in Redis with 24h TTL.
func (s *Service) cacheResult(ctx context.Context, key string, result *TopupHTTPResponse) {
	if err := s.redisCache.Set(ctx, key, "completed", idempotency.DefaultIdempotencyTTL); err != nil {
		slog.Error("failed to cache status in Redis", "key", key, "error", err)
	}

	cached := topupCachedResult{Result: result}
	data, err := json.Marshal(cached)
	if err != nil {
		slog.Error("failed to marshal cached result", "key", key, "error", err)
		return
	}

	cacheKey := key + topupCachedResultSuffix
	if err := s.redisCache.Set(ctx, cacheKey, string(data), idempotency.DefaultIdempotencyTTL); err != nil {
		slog.Error("failed to cache result data in Redis", "key", cacheKey, "error", err)
	}
}

// ── NATS Event ────────────────────────────────────────────────

// natsTopupEvent is the event published to NATS on successful top-up commit.
type natsTopupEvent struct {
	TxID      uuid.UUID `json:"tx_id"`
	UserID    uuid.UUID `json:"user_id"`
	VANumber  string    `json:"va_number"`
	AmountSen int64     `json:"amount_sen"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// publishTopupEvent publishes a top-up completion event to NATS.
func (s *Service) publishTopupEvent(ctx context.Context, vaRecord *VATopupRecord, now time.Time) {
	if s.natsClient == nil || !s.natsClient.IsConnected() {
		slog.Warn("NATS not connected, skipping top-up event publish")
		return
	}

	payload := natsTopupEvent{
		TxID:      *vaRecord.TxLogID,
		UserID:    vaRecord.UserID,
		VANumber:  vaRecord.VANumber,
		AmountSen: vaRecord.AmountSen,
		Status:    types.TxStatusCommitted,
		CreatedAt: now,
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

	slog.Info("published top-up event to NATS",
		"tx_id", payload.TxID, "subject", "tx.completed")
}

// ── BI Limit Check ─────────────────────────────────────────────

// checkBILimit checks if a transaction amount exceeds the BI limit for the user's KYC level.
func checkBILimit(amount types.Money, kycLevel string) *types.DomainError {
	var limit types.Money
	switch kycLevel {
	case types.KYCLevelVerified:
		limit = 1_000_000_000 // Rp 10.000.000 = 1.000.000.000 sen
	default:
		limit = 200_000_000 // Rp 2.000.000 = 200.000.000 sen
	}

	if amount > limit {
		return &types.ErrExceedsTransactionLimit
	}
	return nil
}
