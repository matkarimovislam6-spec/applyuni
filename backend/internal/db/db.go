package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Migrate applies embedded SQL migrations in filename order, once each.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, _ := migrations.ReadFile(name)
		err := pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Tenant runs fn in a transaction that is restricted by row-level security to
// a single organization. Even a buggy query inside fn cannot read or write
// another tenant's rows.
func (d *DB) Tenant(ctx context.Context, orgID uuid.UUID, fn func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE applyuni_app`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.org_id', $1, true)`, orgID.String()); err != nil {
			return err
		}
		return fn(tx)
	})
}
