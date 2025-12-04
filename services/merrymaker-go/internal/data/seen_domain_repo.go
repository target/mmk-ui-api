package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/jackc/pgx/v5"
)

// ErrSeenDomainNotFound is returned when a seen domain record does not exist.
var ErrSeenDomainNotFound = errors.New("seen domain not found")

// SeenDomainRepo implements the SeenDomainRepository interface using PostgreSQL.
type SeenDomainRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewSeenDomainRepo creates a new SeenDomainRepo with the given database connection.
func NewSeenDomainRepo(db *sql.DB) *SeenDomainRepo {
	return &SeenDomainRepo{DB: db, timeProvider: &RealTimeProvider{}}
}

const seenDomainColumns = `id, site_id, domain, scope, first_seen_at, last_seen_at, hit_count, created_at`

// Create inserts a new seen_domains row.
func (r *SeenDomainRepo) Create(
	ctx context.Context,
	req model.CreateSeenDomainRequest,
) (*model.SeenDomain, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	first := r.timeProvider.Now().UTC()
	if req.FirstSeenAt != nil {
		first = req.FirstSeenAt.UTC()
	}
	last := first
	if req.LastSeenAt != nil {
		last = req.LastSeenAt.UTC()
	}

	var out model.SeenDomain
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		row := conn.QueryRow(ctx, `
			INSERT INTO seen_domains (site_id, domain, scope, first_seen_at, last_seen_at, hit_count, created_at)
			VALUES ($1, $2, $3, $4, $5, 1, $6)
			RETURNING `+seenDomainColumns+`
		`, req.SiteID, domain, req.Scope, first, last, r.timeProvider.Now().UTC())
		return row.Scan(&out.ID, &out.SiteID, &out.Domain, &out.Scope, &out.FirstSeenAt, &out.LastSeenAt, &out.HitCount, &out.CreatedAt)
	}); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetByID returns a seen_domains row by ID.
func (r *SeenDomainRepo) GetByID(ctx context.Context, id string) (*model.SeenDomain, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("id is required")
	}
	var out model.SeenDomain
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(
			ctx,
			`SELECT `+seenDomainColumns+` FROM seen_domains WHERE id = $1`,
			id,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		var e error
		out, e = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.SeenDomain])
		return e
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSeenDomainNotFound
		}
		return nil, fmt.Errorf("get seen domain by id: %w", err)
	}
	return &out, nil
}

// List returns seen_domains with simple filters.
func (r *SeenDomainRepo) List(
	ctx context.Context,
	opts model.SeenDomainListOptions,
) ([]*model.SeenDomain, error) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 3)
	next := func() int { return len(args) + 1 }
	if opts.SiteID != nil {
		conditions = append(conditions, fmt.Sprintf("site_id = $%d", next()))
		args = append(args, *opts.SiteID)
	}
	if opts.Scope != nil {
		conditions = append(conditions, fmt.Sprintf("scope = $%d", next()))
		args = append(args, *opts.Scope)
	}
	if opts.Domain != nil {
		// partial match
		conditions = append(conditions, fmt.Sprintf("domain ILIKE $%d", next()))
		args = append(args, "%"+*opts.Domain+"%")
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := `SELECT ` + seenDomainColumns + ` FROM seen_domains ` + where + ` ORDER BY last_seen_at DESC, id DESC`
	var rowsOut []model.SeenDomain
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		rowsOut, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.SeenDomain])
		return err
	}); err != nil {
		return nil, fmt.Errorf("list seen domains: %w", err)
	}
	res := make([]*model.SeenDomain, len(rowsOut))
	for i := range rowsOut {
		res[i] = &rowsOut[i]
	}
	return res, nil
}

// Update updates last_seen_at and/or hit_count by ID.
func (r *SeenDomainRepo) Update(
	ctx context.Context,
	id string,
	req model.UpdateSeenDomainRequest,
) (*model.SeenDomain, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	set := make([]string, 0, 2)
	args := make([]any, 0, 3)
	next := func() int { return len(args) + 1 }
	if req.LastSeenAt != nil {
		set = append(set, fmt.Sprintf("last_seen_at = $%d", next()))
		args = append(args, req.LastSeenAt.UTC())
	}
	if req.HitCount != nil {
		set = append(set, fmt.Sprintf("hit_count = $%d", next()))
		args = append(args, *req.HitCount)
	}
	if len(set) == 0 {
		return r.GetByID(ctx, id)
	}
	args = append(args, id)
	query := "UPDATE seen_domains SET " + strings.Join(
		set,
		", ",
	) + " WHERE id = $" + strconv.Itoa(
		len(args),
	) + " RETURNING " + seenDomainColumns
	var out model.SeenDomain
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var e error
		out, e = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.SeenDomain])
		return e
	}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete deletes a seen_domain by ID.
func (r *SeenDomainRepo) Delete(ctx context.Context, id string) (bool, error) {
	var rows int64
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		ct, err := conn.Exec(ctx, `DELETE FROM seen_domains WHERE id = $1`, id)
		if err != nil {
			return err
		}
		rows = ct.RowsAffected()
		return nil
	}); err != nil {
		return false, fmt.Errorf("delete seen domain: %w", err)
	}
	return rows > 0, nil
}

// Lookup finds a seen_domains row by site+domain+scope.
func (r *SeenDomainRepo) Lookup(
	ctx context.Context,
	req model.SeenDomainLookupRequest,
) (*model.SeenDomain, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	d := strings.ToLower(strings.TrimSpace(req.Domain))
	var out model.SeenDomain
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT `+seenDomainColumns+` FROM seen_domains
			WHERE site_id = $1 AND domain = $2 AND scope = $3
		`, req.SiteID, d, req.Scope)
		if err != nil {
			return err
		}
		defer rows.Close()
		var e error
		out, e = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.SeenDomain])
		return e
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // Not found is valid for cache lookups
		}
		return nil, fmt.Errorf("lookup seen domain: %w", err)
	}
	return &out, nil
}

// RecordSeen performs an upsert for a seen domain: insert if not exists, else bump last_seen_at and hit_count.
func (r *SeenDomainRepo) RecordSeen(
	ctx context.Context,
	req model.RecordDomainSeenRequest,
) (*model.SeenDomain, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	now := r.timeProvider.Now().UTC()
	seenAt := now
	if req.SeenAt != nil {
		seenAt = req.SeenAt.UTC()
	}
	var out model.SeenDomain
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			INSERT INTO seen_domains (site_id, domain, scope, first_seen_at, last_seen_at, hit_count, created_at)
			VALUES ($1, $2, $3, $4, $4, 1, $5)
			ON CONFLICT (site_id, domain, scope)
			DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, hit_count = seen_domains.hit_count + 1
			RETURNING `+seenDomainColumns+`
		`, req.SiteID, req.Domain, req.Scope, seenAt, now)
		if err != nil {
			return err
		}
		defer rows.Close()
		var e error
		out, e = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.SeenDomain])
		return e
	}); err != nil {
		return nil, err
	}
	return &out, nil
}

var _ core.SeenDomainRepository = (*SeenDomainRepo)(nil)
