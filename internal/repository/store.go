package repository

import (
	"context"
	"errors"
	"fmt"
	"plata/internal/model"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (repo *Repository) GetUpdate(ctx context.Context, id string) (model.RateUpdate, error) {
	query := `
		SELECT id, base, quote, status, price, error,
		       requested_at, updated_at, completed_at
		FROM rate_updates
		WHERE id = $1
	`

	var (
		updated model.RateUpdate
		errText *string
	)
	err := repo.db.QueryRow(ctx, query, id).Scan(
		&updated.ID,
		&updated.Pair.Base,
		&updated.Pair.Quote,
		&updated.Status,
		&updated.Price,
		&errText,
		&updated.RequestedAt,
		&updated.UpdatedAt,
		&updated.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RateUpdate{}, model.ErrNotFound
		}

		return model.RateUpdate{}, fmt.Errorf("get rate update: %w", err)
	}

	if errText != nil {
		updated.Error = *errText
	}

	return updated, nil
}

func (repo *Repository) CreatePendingUpdate(ctx context.Context, upd model.RateUpdate) (model.RateUpdate, error) {
	query := `
		INSERT INTO rate_updates (
			id, base, quote, status, requested_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (base, quote)
		WHERE status IN ('pending', 'in_progress')
		DO UPDATE SET updated_at = rate_updates.updated_at
		RETURNING
			id, base, quote, status, price, error,
			requested_at, updated_at, completed_at
	`

	var (
		created model.RateUpdate
		errText *string
	)

	err := repo.db.QueryRow(
		ctx,
		query,
		upd.ID,
		upd.Pair.Base,
		upd.Pair.Quote,
		upd.Status,
		upd.RequestedAt,
		upd.UpdatedAt,
	).Scan(
		&created.ID,
		&created.Pair.Base,
		&created.Pair.Quote,
		&created.Status,
		&created.Price,
		&errText,
		&created.RequestedAt,
		&created.UpdatedAt,
		&created.CompletedAt,
	)
	if err != nil {
		return model.RateUpdate{}, fmt.Errorf("create pending rate update: %w", err)
	}
	if errText != nil {
		created.Error = *errText
	}

	return created, nil
}

func (repo *Repository) ClaimPendingUpdates(ctx context.Context, limit int, now time.Time) ([]model.RateUpdate, error) {
	query := `
	UPDATE rate_updates
	SET status = 'in_progress', updated_at = $1
	WHERE id IN (
		SELECT id FROM rate_updates
		WHERE status = 'pending'
		ORDER BY requested_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	RETURNING id, base, quote, status, price, error,
	          requested_at, updated_at, completed_at
	`

	rows, err := repo.db.Query(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("claim pending updates: %w", err)
	}
	defer rows.Close()

	updates := []model.RateUpdate{}
	for rows.Next() {
		var (
			upd     model.RateUpdate
			errText *string
		)

		if err := rows.Scan(
			&upd.ID,
			&upd.Pair.Base,
			&upd.Pair.Quote,
			&upd.Status,
			&upd.Price,
			&errText,
			&upd.RequestedAt,
			&upd.UpdatedAt,
			&upd.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed update: %w", err)
		}
		if errText != nil {
			upd.Error = *errText
		}

		updates = append(updates, upd)
	}

	return updates, rows.Err()
}

func (repo *Repository) CompleteUpdateWithRate(ctx context.Context, id string, price float64, completedAt time.Time, rate model.Rate) error {
	tx, err := repo.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
	UPDATE rate_updates
	SET status = 'completed', price = $2, completed_at = $3, updated_at = $3
	WHERE id = $1 AND status = 'in_progress'
	`, id, price, completedAt)
	if err != nil {
		return fmt.Errorf("complete update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete update %s: not in progress", id)
	}

	if _, err := tx.Exec(ctx, `
	INSERT INTO rates (base, quote, price, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (base, quote) DO UPDATE
	SET price = $3, updated_at = $4
	WHERE rates.updated_at < $4
	`, rate.Pair.Base, rate.Pair.Quote, rate.Price, rate.UpdatedAt); err != nil {
		return fmt.Errorf("save latest rate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (repo *Repository) FailUpdate(ctx context.Context, id string, errMsg string, now time.Time) error {
	query := `
	UPDATE rate_updates
	SET status = 'failed', error = $2, updated_at = $3
	WHERE id = $1 AND status = 'in_progress'
	`

	_, err := repo.db.Exec(ctx, query, id, errMsg, now)
	if err != nil {
		return fmt.Errorf("fail update: %w", err)
	}

	return nil
}

func (repo *Repository) RequeueUpdate(ctx context.Context, id string, now time.Time) error {
	query := `
	UPDATE rate_updates
	SET status = 'pending', updated_at = $2
	WHERE id = $1 AND status = 'in_progress'
	`

	_, err := repo.db.Exec(ctx, query, id, now)
	if err != nil {
		return fmt.Errorf("requeue update: %w", err)
	}

	return nil
}

func (repo *Repository) GetLatestRate(ctx context.Context, pair model.CurrencyPair) (model.Rate, error) {
	query := `
	SELECT base, quote, price, updated_at
	FROM rates
	WHERE base = $1 AND quote = $2
	`

	var rate model.Rate
	err := repo.db.QueryRow(ctx, query, pair.Base, pair.Quote).Scan(&rate.Pair.Base, &rate.Pair.Quote, &rate.Price, &rate.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Rate{}, model.ErrNotFound
		}

		return model.Rate{}, fmt.Errorf("get latest rate: %w", err)
	}

	return rate, nil
}

func (repo *Repository) ResetStaleUpdates(ctx context.Context, staleAfter time.Duration, now time.Time) (int64, error) {
	query := `
	UPDATE rate_updates
	SET status = 'pending', updated_at = $1
	WHERE status = 'in_progress' AND updated_at < $2
	`

	tag, err := repo.db.Exec(ctx, query, now, now.Add(-staleAfter))
	if err != nil {
		return 0, fmt.Errorf("reset stale updates: %w", err)
	}

	return tag.RowsAffected(), nil
}
