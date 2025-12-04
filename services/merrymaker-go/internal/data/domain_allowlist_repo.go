package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// DomainAllowlistRepo provides database operations for domain allowlists.
type DomainAllowlistRepo struct {
	DB *sql.DB
}

// NewDomainAllowlistRepo creates a new domain allowlist repository.
func NewDomainAllowlistRepo(db *sql.DB) *DomainAllowlistRepo {
	return &DomainAllowlistRepo{DB: db}
}

const domainAllowlistColumns = `id, scope, pattern, pattern_type, description, enabled, priority, created_at, updated_at`

// Create creates a new domain allowlist entry.
func (r *DomainAllowlistRepo) Create(
	ctx context.Context,
	req *model.CreateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	if req == nil {
		return nil, errors.New("create domain allowlist request is required")
	}

	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var allowlist model.DomainAllowlist
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `
			INSERT INTO domain_allowlists (scope, pattern, pattern_type, description, enabled, priority)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING ` + domainAllowlistColumns

		rows, err := conn.Query(ctx, query,
			req.Scope, req.Pattern, req.PatternType,
			req.Description, *req.Enabled, *req.Priority)
		if err != nil {
			return err
		}
		defer rows.Close()

		allowlist, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.DomainAllowlist])
		return err
	})
	if err != nil {
		return nil, err
	}

	return &allowlist, nil
}

// GetByID retrieves a domain allowlist entry by ID.
func (r *DomainAllowlistRepo) GetByID(ctx context.Context, id string) (*model.DomainAllowlist, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("id is required")
	}

	var allowlist model.DomainAllowlist
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `SELECT ` + domainAllowlistColumns + ` FROM domain_allowlists WHERE id = $1`
		rows, err := conn.Query(ctx, query, id)
		if err != nil {
			return err
		}
		defer rows.Close()

		allowlist, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.DomainAllowlist])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}

	return &allowlist, nil
}

// Update updates an existing domain allowlist entry.
//
//nolint:funlen // dynamic SQL builder requires branching; acceptable complexity
func (r *DomainAllowlistRepo) Update(
	ctx context.Context,
	id string,
	req model.UpdateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if strings.TrimSpace(id) == "" {
		return nil, errors.New("id is required")
	}

	// Build dynamic update query
	setParts := []string{"updated_at = NOW()"}
	args := []any{}
	argIndex := 1

	if req.Scope != nil {
		setParts = append(setParts, fmt.Sprintf("scope = $%d", argIndex))
		args = append(args, *req.Scope)
		argIndex++
	}
	if req.Pattern != nil {
		setParts = append(setParts, fmt.Sprintf("pattern = $%d", argIndex))
		args = append(args, *req.Pattern)
		argIndex++
	}
	if req.PatternType != nil {
		setParts = append(setParts, fmt.Sprintf("pattern_type = $%d", argIndex))
		args = append(args, *req.PatternType)
		argIndex++
	}
	if req.Description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *req.Description)
		argIndex++
	}
	if req.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("enabled = $%d", argIndex))
		args = append(args, *req.Enabled)
		argIndex++
	}
	if req.Priority != nil {
		setParts = append(setParts, fmt.Sprintf("priority = $%d", argIndex))
		args = append(args, *req.Priority)
		argIndex++
	}

	// Add ID as the last parameter
	args = append(args, id)

	var allowlist model.DomainAllowlist
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := fmt.Sprintf(`
			UPDATE domain_allowlists
			SET %s
			WHERE id = $%d
			RETURNING `+domainAllowlistColumns,
			strings.Join(setParts, ", "), argIndex)

		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		allowlist, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.DomainAllowlist])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}

	return &allowlist, nil
}

// Delete deletes a domain allowlist entry by ID.
func (r *DomainAllowlistRepo) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("id is required")
	}

	return pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		result, err := conn.Exec(ctx, `DELETE FROM domain_allowlists WHERE id = $1`, id)
		if err != nil {
			return err
		}

		if result.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}

		return nil
	})
}

// List returns domain allowlist entries with filtering options.
//
//nolint:funlen // dynamic filtering + pagination branching; acceptable complexity
func (r *DomainAllowlistRepo) List(
	ctx context.Context,
	opts model.DomainAllowlistListOptions,
) ([]*model.DomainAllowlist, error) {
	conditions := []string{}
	args := []any{}
	argIndex := 1

	// GlobalOnly filter - show only global scope entries
	if opts.GlobalOnly != nil && *opts.GlobalOnly {
		conditions = append(conditions, "scope = 'global'")
	}

	if opts.Scope != nil {
		conditions = append(conditions, fmt.Sprintf("scope = $%d", argIndex))
		args = append(args, *opts.Scope)
		argIndex++
	}

	if opts.Pattern != nil {
		conditions = append(conditions, fmt.Sprintf("pattern ILIKE $%d", argIndex))
		args = append(args, "%"+*opts.Pattern+"%")
		argIndex++
	}

	if opts.PatternType != nil {
		conditions = append(conditions, fmt.Sprintf("pattern_type = $%d", argIndex))
		args = append(args, *opts.PatternType)
		argIndex++
	}

	if opts.Enabled != nil {
		conditions = append(conditions, fmt.Sprintf("enabled = $%d", argIndex))
		args = append(args, *opts.Enabled)
		argIndex++
	}

	// GlobalOnly logic is already handled above

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Add ordering and pagination
	orderClause := "ORDER BY priority ASC, created_at DESC"
	limitClause := ""

	if opts.Limit > 0 {
		limitClause = fmt.Sprintf("LIMIT $%d", argIndex)
		args = append(args, opts.Limit)
		argIndex++

		if opts.Offset > 0 {
			limitClause += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, opts.Offset)
		}
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM domain_allowlists
		%s
		%s
		%s`,
		domainAllowlistColumns, whereClause, orderClause, limitClause)

	var allowlists []*model.DomainAllowlist
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		results, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.DomainAllowlist])
		if err != nil {
			return err
		}

		allowlists = make([]*model.DomainAllowlist, len(results))
		for i := range results {
			allowlists[i] = &results[i]
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return allowlists, nil
}

// GetForScope retrieves all enabled domain allowlist entries for a specific site and scope.
// This includes both site-specific entries and global entries, ordered by priority.
func (r *DomainAllowlistRepo) GetForScope(
	ctx context.Context,
	req model.DomainAllowlistLookupRequest,
) ([]*model.DomainAllowlist, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var allowlists []*model.DomainAllowlist
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		query := `
			SELECT ` + domainAllowlistColumns + `
			FROM domain_allowlists
			WHERE enabled = true
			  AND (scope = $1 OR scope = 'global')
			ORDER BY priority ASC, created_at ASC`

		rows, err := conn.Query(ctx, query, req.Scope)
		if err != nil {
			return err
		}
		defer rows.Close()

		results, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.DomainAllowlist])
		if err != nil {
			return err
		}

		allowlists = make([]*model.DomainAllowlist, len(results))
		for i := range results {
			allowlists[i] = &results[i]
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return allowlists, nil
}

// Stats retrieves statistics about domain allowlist entries.
func (r *DomainAllowlistRepo) Stats(ctx context.Context, siteID *string) (*model.DomainAllowlistStats, error) {
	var stats model.DomainAllowlistStats

	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		whereClause := ""
		args := []any{}

		if siteID != nil {
			whereClause = "WHERE site_id = $1"
			args = append(args, *siteID)
		}

		query := `
			SELECT
				COUNT(*) as total,
				COUNT(CASE WHEN site_id IS NULL THEN 1 END) as global,
				COUNT(CASE WHEN site_id IS NOT NULL THEN 1 END) as scoped,
				COUNT(CASE WHEN enabled = true THEN 1 END) as enabled,
				COUNT(CASE WHEN enabled = false THEN 1 END) as disabled,
				COUNT(CASE WHEN pattern_type = 'exact' THEN 1 END) as exact_count,
				COUNT(CASE WHEN pattern_type = 'wildcard' THEN 1 END) as wildcard_count,
				COUNT(CASE WHEN pattern_type = 'glob' THEN 1 END) as glob_count,
				COUNT(CASE WHEN pattern_type = 'etld_plus_one' THEN 1 END) as etld_count
			FROM domain_allowlists ` + whereClause

		row := conn.QueryRow(ctx, query, args...)
		return row.Scan(
			&stats.Total, &stats.Global, &stats.Scoped,
			&stats.Enabled, &stats.Disabled,
			&stats.ExactCount, &stats.WildcardCount,
			&stats.GlobCount, &stats.ETLDCount,
		)
	})
	if err != nil {
		return nil, err
	}

	return &stats, nil
}
