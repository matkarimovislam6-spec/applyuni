// Package db opens either SQLite (development, single file, no server) or
// PostgreSQL (production) behind database/sql. Application code writes SQL
// with "?" placeholders; they are rewritten to $1, $2… for PostgreSQL.
//
// Tenant isolation: every tenant query filters by org_id in application code.
// On PostgreSQL, row-level security enforces the same rule a second time.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx"
	_ "modernc.org/sqlite"             // driver "sqlite" (pure Go, no cgo)
)

//go:embed migrations
var migrations embed.FS

type Dialect int

const (
	SQLite Dialect = iota
	Postgres
)

func (d Dialect) String() string {
	if d == Postgres {
		return "postgres"
	}
	return "sqlite"
}

type DB struct {
	sql     *sql.DB
	Dialect Dialect
}

// Conn is what queries run on: the database itself or a transaction.
type Conn interface {
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Dialect() Dialect
}

type execer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type conn struct {
	x execer
	d Dialect
}

func (c conn) QueryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return c.x.QueryRowContext(ctx, rebind(c.d, q), args...)
}
func (c conn) Query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return c.x.QueryContext(ctx, rebind(c.d, q), args...)
}
func (c conn) Exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return c.x.ExecContext(ctx, rebind(c.d, q), args...)
}
func (c conn) Dialect() Dialect { return c.d }

// rebind turns "?" placeholders into "$n" for PostgreSQL.
func rebind(d Dialect, q string) string {
	if d != Postgres || !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	n := 0
	for _, ch := range q {
		if ch == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// Open accepts "sqlite:path/to/file.db" or a "postgres://…" URL.
func Open(ctx context.Context, url string) (*DB, error) {
	var d *DB
	switch {
	case strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "postgresql://"):
		s, err := sql.Open("pgx", url)
		if err != nil {
			return nil, err
		}
		d = &DB{sql: s, Dialect: Postgres}
	case strings.HasPrefix(url, "sqlite:"):
		path := strings.TrimPrefix(url, "sqlite:")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_txlock=immediate"
		s, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		// One connection: SQLite allows one writer at a time anyway, and this
		// rules out "database is locked" errors between the API and the worker.
		s.SetMaxOpenConns(1)
		d = &DB{sql: s, Dialect: SQLite}
	default:
		return nil, fmt.Errorf("DATABASE_URL must start with sqlite: or postgres://")
	}
	if err := d.sql.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", d.Dialect, err)
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

// Q runs a query outside a transaction.
func (d *DB) Q() Conn { return conn{x: d.sql, d: d.Dialect} }

// Tx runs fn in a transaction (no tenant scoping: accounts, catalogue, job queue).
func (d *DB) Tx(ctx context.Context, fn func(tx Conn) error) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(conn{x: tx, d: d.Dialect}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Tenant runs fn in a transaction for one organization. Queries inside must
// still filter by org_id; on PostgreSQL row-level security also enforces it.
func (d *DB) Tenant(ctx context.Context, orgID uuid.UUID, fn func(tx Conn) error) error {
	return d.Tx(ctx, func(tx Conn) error {
		if d.Dialect == Postgres {
			if _, err := tx.Exec(ctx, `SET LOCAL ROLE applyuni_app`); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SELECT set_config('app.org_id', ?, true)`, orgID.String()); err != nil {
				return err
			}
		}
		return fn(tx)
	})
}

// Migrate applies the embedded migrations for this dialect, once each, in order.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	dir := "migrations/" + d.Dialect.String()
	names, err := fs.Glob(migrations, dir+"/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		base := filepath.Base(name)
		var n int
		if err := d.Q().QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name = ?`, base).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, _ := migrations.ReadFile(name)
		err := d.Tx(ctx, func(tx Conn) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return fmt.Errorf("%s: %w", base, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES (?)`, base)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// IsUniqueViolation reports whether err is a unique-constraint failure.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "UNIQUE constraint failed") || strings.Contains(s, "SQLSTATE 23505") || strings.Contains(s, "duplicate key")
}
