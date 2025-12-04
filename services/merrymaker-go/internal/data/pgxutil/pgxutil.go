package pgxutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// SQLTxConfig groups parameters for WithSQLTx to keep parameter count  3.
type SQLTxConfig struct {
	Opts *sql.TxOptions
	Fn   func(*sql.Tx) error
}

// TxConfig groups parameters for WithPgxTx to keep parameter count  3.
type TxConfig struct {
	Opts *sql.TxOptions
	Fn   func(pgx.Tx) error
}

// WithSQLTx runs the given function within a database/sql transaction.
func WithSQLTx(ctx context.Context, db *sql.DB, cfg SQLTxConfig) (err error) {
	tx, err := db.BeginTx(ctx, cfg.Opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rerr))
		}
	}()
	if err = cfg.Fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ToPgxTxOptions converts sql.TxOptions to pgx.TxOptions.
func ToPgxTxOptions(opts *sql.TxOptions) pgx.TxOptions {
	var pgxOpts pgx.TxOptions
	if opts == nil {
		return pgxOpts
	}
	pgxOpts.IsoLevel = ToPgxIsoLevel(opts.Isolation)
	pgxOpts.AccessMode = ToPgxAccessMode(opts.ReadOnly)
	return pgxOpts
}

func ToPgxIsoLevel(level sql.IsolationLevel) pgx.TxIsoLevel {
	switch level {
	case sql.LevelDefault:
		return pgx.TxIsoLevel("") // server default
	case sql.LevelSerializable, sql.LevelLinearizable:
		return pgx.Serializable
	case sql.LevelRepeatableRead, sql.LevelSnapshot:
		return pgx.RepeatableRead
	case sql.LevelReadCommitted, sql.LevelWriteCommitted:
		return pgx.ReadCommitted
	case sql.LevelReadUncommitted:
		return pgx.ReadUncommitted
	default:
		return pgx.TxIsoLevel("")
	}
}

func ToPgxAccessMode(readOnly bool) pgx.TxAccessMode {
	if readOnly {
		return pgx.ReadOnly
	}
	return pgx.ReadWrite
}

// WithPgxConn acquires a *pgx.Conn via the stdlib bridge and executes fn with it.
func WithPgxConn(ctx context.Context, db *sql.DB, fn func(*pgx.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get conn from pool: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			// connection close failure is best-effort and ignored
			_ = closeErr
		}
	}()

	return conn.Raw(func(dc any) error {
		std, ok := dc.(*stdlib.Conn)
		if !ok {
			return errors.New("unexpected driver connection type; expected *stdlib.Conn")
		}
		pgxConn := std.Conn()
		return fn(pgxConn)
	})
}

// WithPgxTx runs the given function within a pgx transaction using the stdlib bridge.
func WithPgxTx(ctx context.Context, db *sql.DB, cfg TxConfig) error {
	return WithPgxConn(ctx, db, func(pgxConn *pgx.Conn) error {
		tx, err := pgxConn.BeginTx(ctx, ToPgxTxOptions(cfg.Opts))
		if err != nil {
			return fmt.Errorf("begin pgx tx: %w", err)
		}
		defer func() {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				// rollback failure is safe to ignore here
				_ = rollbackErr
			}
		}()
		if fnErr := cfg.Fn(tx); fnErr != nil {
			return fnErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return fmt.Errorf("commit pgx tx: %w", commitErr)
		}
		return nil
	})
}
