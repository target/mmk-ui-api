package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// IOCRepo implements the IOCRepository interface using PostgreSQL.
type IOCRepo struct {
	DB *sql.DB
}

// NewIOCRepo creates a new IOCRepo instance.
func NewIOCRepo(db *sql.DB) *IOCRepo {
	return &IOCRepo{DB: db}
}

const iocColumns = "id, type, value, enabled, description, created_at, updated_at"

// Create creates a new IOC in the database.
func (r *IOCRepo) Create(ctx context.Context, req model.CreateIOCRequest) (*model.IOC, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Guard against nil Enabled (defensive programming)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var ioc model.IOC
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `
			INSERT INTO iocs (type, value, enabled, description)
			VALUES ($1, $2, $3, $4)
			RETURNING ` + iocColumns

		rows, err := conn.Query(ctx, query, req.Type, req.Value, enabled, req.Description)
		if err != nil {
			return err
		}
		defer rows.Close()
		ioc, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.IOC])
		return err
	})
	if err != nil {
		return nil, r.mapWriteErr(err)
	}

	return &ioc, nil
}

// GetByID retrieves an IOC by its ID.
func (r *IOCRepo) GetByID(ctx context.Context, id string) (*model.IOC, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}

	var ioc model.IOC
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `SELECT ` + iocColumns + ` FROM iocs WHERE id = $1`
		rows, err := conn.Query(ctx, query, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		ioc, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.IOC])
		return err
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIOCNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get IOC by ID: %w", err)
	}

	return &ioc, nil
}

// List retrieves IOCs based on the provided options.
func (r *IOCRepo) List(ctx context.Context, opts model.IOCListOptions) ([]*model.IOC, error) {
	opts.Normalize()

	var iocs []model.IOC
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `SELECT ` + iocColumns + ` FROM iocs WHERE 1=1`
		var args []any

		if opts.Type != nil {
			args = append(args, *opts.Type)
			query += fmt.Sprintf(" AND type = $%d", len(args))
		}
		if opts.Enabled != nil {
			args = append(args, *opts.Enabled)
			query += fmt.Sprintf(" AND enabled = $%d", len(args))
		}
		if opts.Search != nil && *opts.Search != "" {
			args = append(args, "%"+*opts.Search+"%")
			// ILIKE is already case-insensitive; no need for lower()
			query += fmt.Sprintf(" AND value ILIKE $%d", len(args))
		}

		query += " ORDER BY created_at DESC, id DESC"

		if opts.Limit > 0 {
			args = append(args, opts.Limit)
			query += fmt.Sprintf(" LIMIT $%d", len(args))
		}
		if opts.Offset > 0 {
			args = append(args, opts.Offset)
			query += fmt.Sprintf(" OFFSET $%d", len(args))
		}

		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		iocs, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.IOC])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list IOCs: %w", err)
	}

	result := make([]*model.IOC, len(iocs))
	for i := range iocs {
		result[i] = &iocs[i]
	}

	return result, nil
}

// Update updates an existing IOC.
func (r *IOCRepo) Update(ctx context.Context, id string, req model.UpdateIOCRequest) (*model.IOC, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}

	// Get existing IOC to determine final type for validation
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	finalType := existing.Type
	if req.Type != nil {
		finalType = *req.Type
	}

	req.NormalizeWithFinalType(finalType)
	if validateErr := req.Validate(existing.Type); validateErr != nil {
		return nil, validateErr
	}

	var ioc model.IOC
	err = pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query, args := r.buildUpdateQuery(id, req)
		if len(args) == 1 {
			return errors.New("no fields to update")
		}

		rows, queryErr := conn.Query(ctx, query, args...)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		collected, collectErr := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.IOC])
		if collectErr != nil {
			return collectErr
		}
		ioc = collected
		return nil
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIOCNotFound
	}
	if err != nil {
		return nil, r.mapWriteErr(err)
	}

	return &ioc, nil
}

// buildUpdateQuery constructs the UPDATE query and args for an IOC update.
func (r *IOCRepo) buildUpdateQuery(id string, req model.UpdateIOCRequest) (string, []any) {
	query := `UPDATE iocs SET `
	var args []any
	var sets []string

	if req.Type != nil {
		args = append(args, *req.Type)
		sets = append(sets, fmt.Sprintf("type = $%d", len(args)))
	}
	if req.Value != nil {
		args = append(args, *req.Value)
		sets = append(sets, fmt.Sprintf("value = $%d", len(args)))
	}
	if req.Enabled != nil {
		args = append(args, *req.Enabled)
		sets = append(sets, fmt.Sprintf("enabled = $%d", len(args)))
	}
	if req.Description != nil {
		args = append(args, *req.Description)
		sets = append(sets, fmt.Sprintf("description = $%d", len(args)))
	}

	query += strings.Join(sets, ", ")
	args = append(args, id)
	query += fmt.Sprintf(" WHERE id = $%d RETURNING ", len(args)) + iocColumns

	return query, args
}

// Delete removes an IOC by its ID.
// Returns (true, nil) if deleted, (false, nil) if not found, or (false, error) on failure.
func (r *IOCRepo) Delete(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.New("id is required")
	}

	var deleted bool
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		result, err := conn.Exec(ctx, `DELETE FROM iocs WHERE id = $1`, id)
		if err != nil {
			return err
		}
		deleted = result.RowsAffected() > 0
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete IOC: %w", err)
	}

	return deleted, nil
}

// BulkCreate creates multiple IOCs in bulk.
func (r *IOCRepo) BulkCreate(ctx context.Context, req model.BulkCreateIOCsRequest) (int, error) {
	req.Normalize()
	if !req.Type.Valid() {
		return 0, errors.New("invalid ioc type")
	}
	if len(req.Values) == 0 {
		return 0, nil
	}

	// Validate all values before inserting
	for _, v := range req.Values {
		valReq := model.CreateIOCRequest{Type: req.Type, Value: v}
		if err := valReq.Validate(); err != nil {
			return 0, fmt.Errorf("invalid value %q: %w", v, err)
		}
	}

	var count int
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) (err error) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if rerr := tx.Rollback(ctx); rerr != nil && !errors.Is(rerr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rerr))
			}
		}()

		// Use UNNEST for efficient batch insert with ON CONFLICT DO NOTHING
		// RETURNING 1 allows us to count only the inserted rows
		query := `
			INSERT INTO iocs (type, value, enabled, description)
			SELECT $1, v, $2, $3
			FROM UNNEST($4::text[]) AS v
			ON CONFLICT DO NOTHING
			RETURNING 1
		`

		rows, err := tx.Query(ctx, query, req.Type, req.Enabled, req.Description, req.Values)
		if err != nil {
			return err
		}
		defer rows.Close()

		// Count the number of rows returned (= number of rows inserted)
		n := 0
		for rows.Next() {
			n++
		}
		count = n

		return tx.Commit(ctx)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to bulk create IOCs: %w", err)
	}

	return count, nil
}

// LookupHost checks if a host (domain or IP) matches any enabled IOC.
// This is the key method used by the rules engine.
// TODO: Optimize for scale - current implementation loads all enabled IOCs into memory.
// For production with 50k+ IOCs, consider:
// - IPs: Use Postgres inet/cidr types with GiST/btree indexes for efficient range queries.
// - Domains: Pre-filter exact matches in SQL, use suffix indexes for wildcard patterns.
// - Matcher: Move complex matching logic to database-side WHERE clauses where possible.
func (r *IOCRepo) LookupHost(ctx context.Context, req model.IOCLookupRequest) (*model.IOC, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Try to parse as IP first
	if addr, err := netip.ParseAddr(req.Host); err == nil {
		return r.lookupIP(ctx, addr)
	}

	// Otherwise treat as domain
	return r.lookupDomain(ctx, req.Host)
}

// lookupIP checks if an IP address matches any enabled IP IOC (exact or CIDR).
func (r *IOCRepo) lookupIP(ctx context.Context, addr netip.Addr) (*model.IOC, error) {
	var iocs []model.IOC
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `SELECT ` + iocColumns + ` FROM iocs WHERE type = 'ip' AND enabled = true`
		rows, queryErr := conn.Query(ctx, query)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		collected, collectErr := pgx.CollectRows(rows, pgx.RowToStructByName[model.IOC])
		if collectErr != nil {
			return collectErr
		}
		iocs = collected
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to lookup IP IOCs: %w", err)
	}

	// Check each IOC for exact match or CIDR containment
	for i := range iocs {
		ioc := &iocs[i]
		// Try exact IP match
		if checkAddr, parseErr := netip.ParseAddr(ioc.Value); parseErr == nil {
			if checkAddr == addr {
				return ioc, nil
			}
		}
		// Try CIDR match
		if prefix, parseErr := netip.ParsePrefix(ioc.Value); parseErr == nil {
			if prefix.Contains(addr) {
				return ioc, nil
			}
		}
	}

	return nil, ErrIOCNotFound
}

// lookupDomain checks if a domain matches any enabled FQDN IOC (exact or wildcard).
func (r *IOCRepo) lookupDomain(ctx context.Context, domain string) (*model.IOC, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))

	var iocs []model.IOC
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `SELECT ` + iocColumns + ` FROM iocs WHERE type = 'fqdn' AND enabled = true`
		rows, queryErr := conn.Query(ctx, query)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		collected, collectErr := pgx.CollectRows(rows, pgx.RowToStructByName[model.IOC])
		if collectErr != nil {
			return collectErr
		}
		iocs = collected
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to lookup FQDN IOCs: %w", err)
	}

	// Check each IOC for exact or wildcard match
	for i := range iocs {
		ioc := &iocs[i]
		if matchesDomain(domain, ioc.Value) {
			return ioc, nil
		}
	}

	return nil, ErrIOCNotFound
}

// matchesDomain checks if a domain matches a pattern (exact or wildcard).
// Supports patterns like "evil.com", "*.evil.com", "malware.*.com".
// Note: This is label-wise wildcard matching - requires equal number of labels.
// Pattern "*.evil.com" matches "sub.evil.com" but NOT "a.b.evil.com".
// This is intentional for precise IOC matching; adjust if broader suffix matching is needed.
func matchesDomain(domain, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))

	// Exact match
	if domain == pattern {
		return true
	}

	// Wildcard matching
	if !strings.Contains(pattern, "*") {
		return false
	}

	domainLabels := strings.Split(domain, ".")
	patternLabels := strings.Split(pattern, ".")

	if len(domainLabels) != len(patternLabels) {
		return false
	}

	for i := range patternLabels {
		if patternLabels[i] == "*" {
			continue
		}
		if domainLabels[i] != patternLabels[i] {
			return false
		}
	}

	return true
}

// Stats retrieves statistics about IOCs.
func (r *IOCRepo) Stats(ctx context.Context) (*core.IOCStats, error) {
	var stats core.IOCStats
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `
			SELECT
				COUNT(*) AS total_count,
				COUNT(*) FILTER (WHERE enabled = true) AS enabled_count,
				COUNT(*) FILTER (WHERE type = 'fqdn') AS fqdn_count,
				COUNT(*) FILTER (WHERE type = 'ip') AS ip_count
			FROM iocs
		`
		row := conn.QueryRow(ctx, query)
		return row.Scan(&stats.TotalCount, &stats.EnabledCount, &stats.FQDNCount, &stats.IPCount)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get IOC stats: %w", err)
	}

	return &stats, nil
}

// mapWriteErr maps database errors to domain-specific errors.
func (r *IOCRepo) mapWriteErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrIOCAlreadyExists
	}
	return err
}

// Ensure IOCRepo implements the IOCRepository interface.
var _ core.IOCRepository = (*IOCRepo)(nil)
