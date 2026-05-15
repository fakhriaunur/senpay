package ledger

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTxLogStore implements LedgerStore backed by PostgreSQL.
type PostgresTxLogStore struct {
	pool *pgxpool.Pool
}

// NewPostgresTxLogStore creates a new PostgresTxLogStore.
func NewPostgresTxLogStore(pool *pgxpool.Pool) *PostgresTxLogStore {
	return &PostgresTxLogStore{pool: pool}
}

// Append inserts a new transaction log entry.
func (s *PostgresTxLogStore) Append(ctx context.Context, tx types.Transaction) error {
	const query = `
		INSERT INTO tx_log (
			id, idempotency_key, tx_type, sender_id, receiver_id,
			amount_sen, currency, status, failure_reason, category, created_at, committed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	cat := tx.Category
	if cat == "" {
		cat = types.CategoryDefault
	}
	_, err := s.pool.Exec(ctx, query,
		tx.ID, tx.IdempotencyKey, tx.TxType,
		tx.SenderID, tx.ReceiverID,
		tx.AmountSen, tx.Currency, tx.Status,
		tx.FailureReason, cat, tx.CreatedAt, tx.CommittedAt,
	)
	if err != nil {
		// Check for unique constraint violation on idempotency_key.
		if isUniqueViolation(err) {
			return fmt.Errorf("duplicate idempotency key: %s", tx.IdempotencyKey)
		}
		return fmt.Errorf("insert tx_log: %w", err)
	}
	return nil
}

// QueryByUserID retrieves transaction log entries for a user with cursor-based pagination.
func (s *PostgresTxLogStore) QueryByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]types.Transaction, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var rows pgx.Rows
	var err error

	if cursor == "" {
		// First page: fetch most recent items.
		const query = `
			SELECT id, idempotency_key, tx_type, sender_id, receiver_id,
				amount_sen, currency, status, failure_reason, created_at, committed_at
			FROM tx_log
			WHERE sender_id = $1 OR receiver_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		`
		rows, err = s.pool.Query(ctx, query, userID, limit+1) // fetch one extra to detect if more exist
	} else {
		// Decode cursor: base64(created_at_unix_ns,id).
		cursorCreatedAt, cursorID, decodeErr := decodeCursor(cursor)
		if decodeErr != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", decodeErr)
		}
		cursorTime := time.Unix(0, cursorCreatedAt).UTC()
		const query = `
			SELECT id, idempotency_key, tx_type, sender_id, receiver_id,
				amount_sen, currency, status, failure_reason, created_at, committed_at
			FROM tx_log
			WHERE (sender_id = $1 OR receiver_id = $1)
			  AND (created_at < $2 OR (created_at = $2 AND id < $3))
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`
		rows, err = s.pool.Query(ctx, query, userID, cursorTime, cursorID, limit+1)
	}
	if err != nil {
		return nil, "", fmt.Errorf("query tx_log: %w", err)
	}
	defer rows.Close()

	var txs []types.Transaction
	for rows.Next() {
		var t types.Transaction
		err := rows.Scan(
			&t.ID, &t.IdempotencyKey, &t.TxType, &t.SenderID, &t.ReceiverID,
			&t.AmountSen, &t.Currency, &t.Status, &t.FailureReason, &t.CreatedAt, &t.CommittedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("scan tx_log row: %w", err)
		}
		txs = append(txs, t)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows iteration: %w", err)
	}

	// Determine next cursor.
	var nextCursor string
	if len(txs) > limit {
		// We fetched one extra row — there are more results.
		last := txs[limit-1]
		nextCursor = encodeCursor(last.CreatedAt.UnixNano(), last.ID)
		txs = txs[:limit] // truncate to requested limit
	}

	return txs, nextCursor, nil
}

// encodeCursor creates an opaque cursor string from a timestamp and UUID.
func encodeCursor(unixNano int64, id uuid.UUID) string {
	data := fmt.Sprintf("%d,%s", unixNano, id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(data))
}

// decodeCursor parses a cursor string into a timestamp and UUID.
func decodeCursor(cursor string) (int64, uuid.UUID, error) {
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, uuid.Nil, fmt.Errorf("base64 decode: %w", err)
	}
	parts := strings.SplitN(string(data), ",", 2)
	if len(parts) != 2 {
		return 0, uuid.Nil, errors.New("invalid cursor format")
	}
	unixNano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, uuid.Nil, fmt.Errorf("parse timestamp: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return 0, uuid.Nil, fmt.Errorf("parse uuid: %w", err)
	}
	return unixNano, id, nil
}

// FindByID retrieves a single transaction log entry by its ID.
func (s *PostgresTxLogStore) FindByID(ctx context.Context, id uuid.UUID) (types.Transaction, error) {
	const query = `
		SELECT id, idempotency_key, tx_type, sender_id, receiver_id,
			amount_sen, currency, status, failure_reason, created_at, committed_at
		FROM tx_log
		WHERE id = $1
	`
	var t types.Transaction
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.IdempotencyKey, &t.TxType, &t.SenderID, &t.ReceiverID,
		&t.AmountSen, &t.Currency, &t.Status, &t.FailureReason, &t.CreatedAt, &t.CommittedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Transaction{}, types.ErrUserNotFound
		}
		return types.Transaction{}, fmt.Errorf("find tx_log by id: %w", err)
	}
	return t, nil
}

// isUniqueViolation checks if a PostgreSQL error is a unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
