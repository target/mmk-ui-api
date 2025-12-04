package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// JobResultRepo provides persistence for job execution results.
type JobResultRepo struct {
	DB *sql.DB
}

// NewJobResultRepo constructs a JobResultRepo.
func NewJobResultRepo(db *sql.DB) *JobResultRepo {
	return &JobResultRepo{DB: db}
}

// Upsert stores or updates job results for a given job.
func (r *JobResultRepo) Upsert(ctx context.Context, params core.UpsertJobResultParams) error {
	if r == nil || r.DB == nil {
		return ErrJobResultsNotConfigured
	}
	if params.JobID == "" {
		return ErrJobIDRequired
	}
	const query = `
		INSERT INTO job_results (job_id, job_type, result, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (job_id)
		DO UPDATE SET
			job_type = EXCLUDED.job_type,
			result = EXCLUDED.result,
			updated_at = now();`
	if _, err := r.DB.ExecContext(ctx, query, params.JobID, params.JobType, params.Result); err != nil {
		return fmt.Errorf("upsert job_results: %w", err)
	}
	return nil
}

// GetByJobID retrieves job results for a given job ID.
func (r *JobResultRepo) GetByJobID(ctx context.Context, jobID string) (*model.JobResult, error) {
	if r == nil || r.DB == nil {
		return nil, ErrJobResultsNotConfigured
	}
	if jobID == "" {
		return nil, ErrJobIDRequired
	}

	const query = `
		SELECT job_id, job_type, result, created_at, updated_at
		FROM job_results
		WHERE job_id = $1`

	var res *model.JobResult
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, jobID)
		if err != nil {
			return err
		}
		defer rows.Close()
		result, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.JobResult])
		if err != nil {
			return err
		}
		res = &result
		return nil
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobResultsNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job_results: %w", err)
	}
	return res, nil
}

// ListByAlertID retrieves job results associated with a given alert ID (via stored JSON payload).
func (r *JobResultRepo) ListByAlertID(ctx context.Context, alertID string) ([]*model.JobResult, error) {
	if r == nil || r.DB == nil {
		return nil, ErrJobResultsNotConfigured
	}
	if strings.TrimSpace(alertID) == "" {
		return nil, ErrAlertIDRequired
	}

	const query = `
		SELECT job_id, job_type, result, created_at, updated_at
		FROM job_results
		WHERE job_type = $1
			AND result ->> 'alert_id' = $2
		ORDER BY updated_at DESC`

	var rowsOut []*model.JobResult
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, model.JobTypeAlert, alertID)
		if err != nil {
			return err
		}
		defer rows.Close()

		collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.JobResult])
		if err != nil {
			return err
		}
		for i := range collected {
			row := collected[i]
			rowsOut = append(rowsOut, &row)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list job_results: %w", err)
	}
	return rowsOut, nil
}
