package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	connectAttempts = 10
	connectPause    = time.Second
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		lastErr = err

		if attempt == connectAttempts {
			break
		}

		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(connectPause):
		}
	}

	pool.Close()
	return nil, fmt.Errorf("database not ready after %d attempts: %w", connectAttempts, lastErr)
}
