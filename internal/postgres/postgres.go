package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Conn wraps a PostgreSQL connection pool for chatlog data storage.
type Conn struct {
	pool *pgxpool.Pool
}

// New creates a new Postgres connection from the given URL.
func New(ctx context.Context, url string) (*Conn, error) {
	if url == "" {
		return nil, fmt.Errorf("postgres URL is required")
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse postgres URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	c := &Conn{pool: pool}
	if err := c.Migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return c, nil
}

// Close closes the connection pool.
func (c *Conn) Close() {
	if c.pool != nil {
		c.pool.Close()
		c.pool = nil
	}
}

// Pool returns the underlying connection pool for use by the repository.
func (c *Conn) Pool() *pgxpool.Pool {
	return c.pool
}
