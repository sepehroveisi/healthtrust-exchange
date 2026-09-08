package authorityledger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const bookkeepingSQL = `CREATE TABLE IF NOT EXISTS public.authority_ledger_migration_versions (
version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`

type Migrator struct {
	pool      *pgxpool.Pool
	directory string
}

func NewMigrator(pool *pgxpool.Pool, directory string) *Migrator {
	return &Migrator{pool: pool, directory: directory}
}

func (m *Migrator) Up(ctx context.Context) error {
	if _, err := m.pool.Exec(ctx, bookkeepingSQL); err != nil {
		return fmt.Errorf("create migration bookkeeping: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(m.directory, "*.up.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, path := range files {
		version, err := migrationVersion(path)
		if err != nil {
			return err
		}
		var applied bool
		err = m.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.authority_ledger_migration_versions WHERE version=$1)`, version).Scan(&applied)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tx, err := m.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO public.authority_ledger_migration_versions(version) VALUES($1)`, version)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) DownAll(ctx context.Context) error {
	if _, err := m.pool.Exec(ctx, bookkeepingSQL); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(m.directory, "*.down.sql"))
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, path := range files {
		version, err := migrationVersion(path)
		if err != nil {
			return err
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tx, err := m.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM public.authority_ledger_migration_versions WHERE version=$1`, version)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func migrationVersion(path string) (int64, error) {
	name := filepath.Base(path)
	prefix := strings.SplitN(name, "_", 2)[0]
	v, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid migration filename %s", name)
	}
	return v, nil
}
