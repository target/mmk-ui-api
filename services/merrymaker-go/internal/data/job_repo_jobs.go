package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// insertJobParams groups parameters for inserting a job within a transaction.
type insertJobParams struct {
	Req        *model.CreateJobRequest
	Payload    []byte
	Meta       []byte
	MaxRetries int
}

const defaultRetryDelaySeconds = 30

func (r *JobRepo) retryDelay() int {
	if r.cfg.RetryDelaySeconds > 0 {
		return r.cfg.RetryDelaySeconds
	}
	return defaultRetryDelaySeconds
}

func (r *JobRepo) updateJobMetaStatus(ctx context.Context, id string, status model.JobStatus) error {
	if strings.TrimSpace(id) == "" || !status.Valid() {
		return nil
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO job_meta (job_id, last_status, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (job_id) DO UPDATE
		SET last_status = EXCLUDED.last_status,
		    updated_at = now()
	`, id, status)
	if err != nil {
		return fmt.Errorf("update job_meta status: %w", err)
	}
	return nil
}

// SQL used by ReserveNext to atomically reserve the next job.
const reserveNextUpdateSQL = `
  WITH cte AS (
    SELECT id FROM jobs
    WHERE type = $1 AND status = 'pending' AND scheduled_at <= $2
    ORDER BY priority DESC, scheduled_at ASC, created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
  )
  UPDATE jobs j
  SET
    status = 'running',
    started_at = COALESCE(j.started_at, $3),
    lease_expires_at = $4,
    updated_at = $5
  FROM cte
  WHERE j.id = cte.id
  RETURNING j.id, j.type, j.status, j.priority, j.payload, j.metadata, j.session_id, j.site_id, j.source_id, j.is_test, j.scheduled_at, j.started_at, j.completed_at, j.retry_count, j.max_retries, j.last_error, j.lease_expires_at, j.created_at, j.updated_at`

// Create creates a new job in the database with the given parameters.
func (r *JobRepo) Create(
	ctx context.Context,
	req *model.CreateJobRequest,
) (*model.Job, error) {
	if req == nil {
		return nil, errors.New("create job request is required")
	}

	if validateErr := req.Validate(); validateErr != nil {
		return nil, validateErr
	}

	payload, meta, maxRetries, err := r.prepareJobData(req)
	if err != nil {
		return nil, err
	}

	p := &insertJobParams{
		Req:        req,
		Payload:    payload,
		Meta:       meta,
		MaxRetries: maxRetries,
	}

	var job *model.Job
	if txErr := pgxutil.WithPgxTx(ctx, r.DB, pgxutil.TxConfig{
		Fn: func(tx pgx.Tx) error {
			var insertErr error
			job, insertErr = r.insertJobInTx(ctx, tx, p)
			return insertErr
		},
	}); txErr != nil {
		return nil, txErr
	}

	return job, nil
}

// CreateInTx inserts a job within an existing SQL transaction.
func (r *JobRepo) CreateInTx(
	ctx context.Context,
	sqlTx *sql.Tx,
	req *model.CreateJobRequest,
) (*model.Job, error) {
	if sqlTx == nil {
		return nil, errors.New("transaction is required")
	}
	if req == nil {
		return nil, errors.New("create job request is required")
	}
	if validateErr := req.Validate(); validateErr != nil {
		return nil, validateErr
	}

	payload, meta, maxRetries, prepErr := r.prepareJobData(req)
	if prepErr != nil {
		return nil, prepErr
	}

	params := &insertJobParams{
		Req:        req,
		Payload:    payload,
		Meta:       meta,
		MaxRetries: maxRetries,
	}

	query, args := r.buildInsertQuery(params)
	row := sqlTx.QueryRowContext(ctx, query, args...)

	job, scanErr := scanJobFromRow(row)
	if scanErr != nil {
		return nil, fmt.Errorf("collect job: %w", scanErr)
	}

	channel := "job_added_" + string(req.Type)
	if _, notifyErr := sqlTx.ExecContext(ctx, `SELECT pg_notify($1::text, $2::text)`, channel, job.ID); notifyErr != nil {
		return nil, fmt.Errorf("send job notification: %w", notifyErr)
	}

	return job, nil
}

// prepareJobData prepares the payload, metadata, and maxRetries for job creation.
func (r *JobRepo) prepareJobData(req *model.CreateJobRequest) ([]byte, []byte, int, error) {
	if req == nil {
		return nil, nil, 0, errors.New("create job request is required")
	}

	payload, err := json.Marshal(req.Payload)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	meta := []byte(`{}`)
	if req.Metadata != nil {
		meta, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	maxRetries := 3
	if req.IsTest && req.MaxRetries <= 0 {
		maxRetries = 0
	} else if req.MaxRetries > 0 {
		maxRetries = req.MaxRetries
	}

	return payload, meta, maxRetries, nil
}

// insertJobInTx inserts a job within a pgx.Tx and returns the created job.
func (r *JobRepo) insertJobInTx(ctx context.Context, tx pgx.Tx, params *insertJobParams) (*model.Job, error) {
	if params == nil || params.Req == nil {
		return nil, errors.New("insert job params are required")
	}

	query, args := r.buildInsertQuery(params)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}
	job, collectErr := collectJobFromRows(rows)
	rows.Close()
	if collectErr != nil {
		return nil, fmt.Errorf("collect job: %w", collectErr)
	}

	channel := "job_added_" + string(params.Req.Type)
	if _, execErr := tx.Exec(ctx, `SELECT pg_notify($1::text, $2::text)`, channel, job.ID); execErr != nil {
		return nil, fmt.Errorf("send job notification: %w", execErr)
	}

	return job, nil
}

// buildInsertQuery builds an INSERT statement for a job based on the provided parameters.
func (r *JobRepo) buildInsertQuery(p *insertJobParams) (string, []any) {
	if p == nil || p.Req == nil {
		return "", nil
	}

	query := `
      INSERT INTO jobs(type, status, priority, payload, metadata, session_id, site_id, source_id, is_test, scheduled_at, max_retries)
      VALUES ($1,'pending',$2,$3,$4,$5,$6,$7,$8,$9,$10)
      RETURNING ` + jobColumns

	var scheduledAt time.Time
	if p.Req.ScheduledAt != nil {
		scheduledAt = p.Req.ScheduledAt.UTC()
	} else {
		scheduledAt = r.timeProvider.Now().UTC()
	}

	args := []any{
		p.Req.Type,
		p.Req.Priority,
		p.Payload,
		p.Meta,
		p.Req.SessionID,
		p.Req.SiteID,
		p.Req.SourceID,
		p.Req.IsTest,
		scheduledAt,
		p.MaxRetries,
	}
	return query, args
}

// collectJobFromRows collects a single job from pgx rows using pgx v5 helpers.
func collectJobFromRows(rows pgx.Rows) (*model.Job, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}

	job, err := scanJobFromRow(rows)
	if err != nil {
		return nil, err
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	return job, nil
}

type jobRowScanner interface {
	Scan(dest ...any) error
}

type jobRowData struct {
	payload, metadata                      []byte
	sessionID, siteID, sourceID, lastError sql.NullString
	startedAt, completedAt, leaseExpiresAt sql.NullTime
}

func (d *jobRowData) scanInto(scanner jobRowScanner, job *model.Job) error {
	return scanner.Scan(
		&job.ID,
		&job.Type,
		&job.Status,
		&job.Priority,
		&d.payload,
		&d.metadata,
		&d.sessionID,
		&d.siteID,
		&d.sourceID,
		&job.IsTest,
		&job.ScheduledAt,
		&d.startedAt,
		&d.completedAt,
		&job.RetryCount,
		&job.MaxRetries,
		&d.lastError,
		&d.leaseExpiresAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
}

func (d *jobRowData) apply(job *model.Job) {
	job.Payload = cloneJSON(d.payload)
	job.Metadata = cloneJSON(d.metadata)
	job.SessionID = cloneNullableString(d.sessionID)
	job.SiteID = cloneNullableString(d.siteID)
	job.SourceID = cloneNullableString(d.sourceID)
	job.LastError = cloneNullableString(d.lastError)
	job.StartedAt = cloneNullableTime(d.startedAt)
	job.CompletedAt = cloneNullableTime(d.completedAt)
	job.LeaseExpiresAt = cloneNullableTime(d.leaseExpiresAt)
}

func scanJobFromRow(scanner jobRowScanner) (*model.Job, error) {
	job := &model.Job{}
	var data jobRowData
	if err := data.scanInto(scanner, job); err != nil {
		return nil, err
	}

	data.apply(job)
	return job, nil
}

func cloneJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneNullableString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func cloneNullableTime(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	t := nt.Time.UTC()
	return &t
}

// Advisory lock namespace for requeueExpired to avoid cross-job-type contention.
const advisoryLockRequeueMajor int64 = 1001

func advisoryLockRequeueMinor(jobType model.JobType) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(jobType))
	hashValue := h.Sum32()
	maxInt32 := uint32(math.MaxInt32)
	if hashValue > maxInt32 {
		hashValue &= maxInt32
	}
	return int64(hashValue)
}

// requeueExpired requeues expired jobs of the given type and returns the number of jobs requeued.
func (r *JobRepo) requeueExpired(ctx context.Context, jobType model.JobType) (int64, error) {
	var rowsAffected int64
	err := pgxutil.WithSQLTx(ctx, r.DB, pgxutil.SQLTxConfig{
		Fn: func(tx *sql.Tx) error {
			var locked bool
			minorKey := advisoryLockRequeueMinor(jobType)
			if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1::integer, $2::integer)", advisoryLockRequeueMajor, minorKey).Scan(&locked); err != nil {
				return fmt.Errorf("acquire advisory lock: %w", err)
			}
			if !locked {
				rowsAffected = 0
				return nil
			}

			currentTime := r.timeProvider.Now()
			res, err := tx.ExecContext(ctx, `
          UPDATE jobs
          SET status = 'pending', lease_expires_at = NULL
          WHERE type = $1 AND status = 'running'
            AND lease_expires_at IS NOT NULL
            AND lease_expires_at < $2
        `, jobType, currentTime.UTC())
			if err != nil {
				return fmt.Errorf("requeue expired: %w", err)
			}
			ra, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows affected: %w", err)
			}
			rowsAffected = ra
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

// ReserveNext reserves the next available job of the given type for processing.
func (r *JobRepo) ReserveNext(
	ctx context.Context,
	jobType model.JobType,
	leaseSeconds int,
) (*model.Job, error) {
	if !jobType.Valid() {
		return nil, fmt.Errorf("invalid job type: %s", jobType)
	}

	if _, err := r.requeueExpired(ctx, jobType); err != nil {
		return nil, fmt.Errorf("requeue expired jobs: %w", err)
	}

	var job *model.Job
	err := pgxutil.WithPgxTx(ctx, r.DB, pgxutil.TxConfig{
		Opts: &sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
			ReadOnly:  false,
		},
		Fn: func(tx pgx.Tx) error {
			currentTime := r.timeProvider.Now()
			leaseExpiresAt := currentTime.Add(time.Duration(leaseSeconds) * time.Second)

			rows, qerr := tx.Query(
				ctx,
				reserveNextUpdateSQL,
				jobType,
				currentTime.UTC(),
				currentTime.UTC(),
				leaseExpiresAt.UTC(),
				currentTime.UTC(),
			)
			if qerr != nil {
				return fmt.Errorf("reserve job: %w", qerr)
			}
			defer rows.Close()

			j, cerr := collectJobFromRows(rows)
			if errors.Is(cerr, pgx.ErrNoRows) {
				return model.ErrNoJobsAvailable
			}
			if cerr != nil {
				return fmt.Errorf("reserve job: %w", cerr)
			}
			job = j
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, model.ErrNoJobsAvailable) {
			return nil, model.ErrNoJobsAvailable
		}
		return nil, err
	}
	return job, nil
}

// Heartbeat refreshes the lease on a running job.
func (r *JobRepo) Heartbeat(ctx context.Context, jobID string, leaseSeconds int) (bool, error) {
	if leaseSeconds <= 0 {
		return false, errors.New("leaseSeconds must be positive")
	}

	currentTime := r.timeProvider.Now().UTC()
	leaseExpiration := currentTime.Add(time.Duration(leaseSeconds) * time.Second)

	query := `
		UPDATE jobs
		SET lease_expires_at = $2,
		    updated_at = $3
		WHERE id = $1 AND status = 'running'
	`

	res, err := r.DB.ExecContext(ctx, query, jobID, leaseExpiration, currentTime)
	if err != nil {
		return false, fmt.Errorf("heartbeat job: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("heartbeat rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return false, nil
	}

	return true, nil
}

// Complete marks a job as completed successfully.
func (r *JobRepo) Complete(ctx context.Context, id string) (bool, error) {
	currentTime := r.timeProvider.Now().UTC()

	query := `
		UPDATE jobs
		SET status = 'completed',
		    completed_at = $2,
		    updated_at = $3,
		    lease_expires_at = NULL,
		    last_error = NULL
		WHERE id = $1 AND status = 'running'
		RETURNING metadata->>'scheduler.task_name', metadata->>'scheduler.fire_key'
	`

	var taskName, fireKey sql.NullString
	if err := r.DB.QueryRowContext(ctx, query, id, currentTime, currentTime).Scan(&taskName, &fireKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to complete job: %w", err)
	}

	if !taskName.Valid || !fireKey.Valid {
		return true, nil
	}

	if err := r.clearActiveFireKey(ctx, taskName.String, fireKey.String); err != nil {
		r.logClearFireKeyError(
			ctx,
			clearFireKeyErrorParams{
				taskName: taskName.String,
				fireKey:  fireKey.String,
				err:      err,
			},
		)
	}

	if err := r.updateJobMetaStatus(ctx, id, model.JobStatusCompleted); err != nil && r.logger != nil {
		r.logger.WarnContext(ctx, "update job_meta status failed",
			"job_id", id,
			"status", model.JobStatusCompleted,
			"error", err,
		)
	}

	return true, nil
}

// Fail marks a job as failed with the given error message.
func (r *JobRepo) Fail(ctx context.Context, id, errMsg string) (bool, error) {
	retryDelay := r.retryDelay()
	currentTime := r.timeProvider.Now()
	retryScheduledAt := currentTime.Add(time.Duration(retryDelay) * time.Second)

	query := `
      UPDATE jobs
      SET
        last_error = $2,
        retry_count = retry_count + 1,
        status = CASE WHEN retry_count + 1 >= max_retries THEN 'failed' ELSE 'pending' END,
        completed_at = CASE WHEN retry_count + 1 >= max_retries THEN $3::timestamptz ELSE NULL END,
        lease_expires_at = NULL,
        scheduled_at = CASE WHEN retry_count + 1 >= max_retries THEN scheduled_at
                            ELSE $4::timestamptz END,
        updated_at = $5
      WHERE id = $1 AND status = 'running'
      RETURNING status, metadata->>'scheduler.task_name', metadata->>'scheduler.fire_key'
    `

	var status string
	var taskName, fireKey sql.NullString
	if err := r.DB.QueryRowContext(ctx, query, id, errMsg, currentTime.UTC(), retryScheduledAt.UTC(), currentTime.UTC()).Scan(&status, &taskName, &fireKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("fail job: %w", err)
	}

	if status != string(model.JobStatusFailed) || !taskName.Valid || !fireKey.Valid {
		return true, nil
	}

	if err := r.clearActiveFireKey(ctx, taskName.String, fireKey.String); err != nil {
		r.logClearFireKeyError(
			ctx,
			clearFireKeyErrorParams{
				taskName: taskName.String,
				fireKey:  fireKey.String,
				err:      err,
			},
		)
	}

	if err := r.updateJobMetaStatus(ctx, id, model.JobStatus(status)); err != nil && r.logger != nil {
		r.logger.WarnContext(ctx, "update job_meta status failed",
			"job_id", id,
			"status", status,
			"error", err,
		)
	}

	return true, nil
}

func (r *JobRepo) clearActiveFireKey(ctx context.Context, taskName, fireKey string) error {
	if strings.TrimSpace(taskName) == "" || strings.TrimSpace(fireKey) == "" {
		return nil
	}

	query := `
		UPDATE scheduled_jobs
		SET active_fire_key = NULL,
		    active_fire_key_set_at = NULL,
		    updated_at = $3
		WHERE task_name = $1
		  AND active_fire_key = $2
	`

	if _, err := r.DB.ExecContext(ctx, query, taskName, fireKey, r.timeProvider.Now().UTC()); err != nil {
		return fmt.Errorf("clear active fire key: %w", err)
	}
	return nil
}

type clearFireKeyErrorParams struct {
	taskName string
	fireKey  string
	err      error
}

func (r *JobRepo) logClearFireKeyError(ctx context.Context, params clearFireKeyErrorParams) {
	if r.logger == nil {
		return
	}

	r.logger.ErrorContext(ctx, "clear active fire key failed",
		"task_name", params.taskName,
		"fire_key", params.fireKey,
		"error", params.err,
	)
}

// Stats returns statistics about jobs of the given type in different states.
func (r *JobRepo) Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error) {
	var s model.JobStats
	err := r.DB.QueryRowContext(ctx, `
  SELECT
    count(*) FILTER (WHERE status = 'pending')   AS pending,
    count(*) FILTER (WHERE status = 'running')   AS running,
    count(*) FILTER (WHERE status = 'completed') AS completed,
    count(*) FILTER (WHERE status = 'failed')    AS failed
  FROM jobs
  WHERE type = $1
  `, jobType).Scan(
		&s.Pending,
		&s.Running,
		&s.Completed,
		&s.Failed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get job stats: %w", err)
	}
	return &s, nil
}

// WaitForNotification waits for a PostgreSQL notification indicating new jobs are available.
func (r *JobRepo) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	conn, err := r.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get conn from pool: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			_ = cerr
		}
	}()

	channel := "job_added_" + string(jobType)
	quoted := pgx.Identifier{channel}.Sanitize()

	if _, execErr := conn.ExecContext(ctx, "LISTEN "+quoted); execErr != nil {
		return fmt.Errorf("listen %s: %w", channel, execErr)
	}
	defer func() {
		if _, execErr := conn.ExecContext(context.Background(), "UNLISTEN "+quoted); execErr != nil {
			_ = execErr
		}
	}()

	return conn.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			return errors.New("unexpected driver connection type; expected *stdlib.Conn")
		}
		_, notifyErr := sc.Conn().WaitForNotification(ctx)
		return notifyErr
	})
}

// GetByID retrieves a job by its ID.
func (r *JobRepo) GetByID(ctx context.Context, id string) (*model.Job, error) {
	var job *model.Job
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, `
			SELECT `+jobColumns+`
			FROM jobs
			WHERE id = $1
		`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		job, err = collectJobFromRows(rows)
		return err
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return job, nil
}

// RunningJobExistsByTaskName checks if there is a running job for the given scheduler task.
func (r *JobRepo) RunningJobExistsByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (bool, error) {
	mask, err := r.JobStatesByTaskName(ctx, taskName, now)
	if err != nil {
		return false, err
	}
	return mask.Has(domain.OverrunStateRunning), nil
}

// JobStatesByTaskName returns a bitmask describing which overrun states currently exist for a scheduler task.
func (r *JobRepo) JobStatesByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (domain.OverrunStateMask, error) {
	query := `
		SELECT
			COALESCE(bool_or(status = 'running' AND lease_expires_at > $1), FALSE) AS has_running,
			COALESCE(bool_or(status = 'pending'), FALSE) AS has_pending,
			COALESCE(bool_or(status = 'pending' AND COALESCE(retry_count, 0) > 0), FALSE) AS has_retrying
		FROM jobs
		WHERE metadata->>'scheduler.task_name' = $2
		  AND status IN ('running', 'pending')
	`

	var hasRunning, hasPending, hasRetrying bool
	if err := r.DB.QueryRowContext(ctx, query, now.UTC(), taskName).Scan(&hasRunning, &hasPending, &hasRetrying); err != nil {
		return 0, fmt.Errorf("check job states by task name: %w", err)
	}

	var mask domain.OverrunStateMask
	if hasRunning {
		mask |= domain.OverrunStateRunning
	}
	if hasPending {
		mask |= domain.OverrunStatePending
	}
	if hasRetrying {
		mask |= domain.OverrunStateRetrying
	}

	return mask, nil
}

// Delete safely deletes a job by ID with state machine safety checks.
func (r *JobRepo) Delete(ctx context.Context, id string) error {
	currentTime := r.timeProvider.Now()
	res, err := r.DB.ExecContext(ctx, `
		DELETE FROM jobs
		WHERE id = $1
		  AND status IN ('pending', 'completed', 'failed')
		  AND (lease_expires_at IS NULL OR lease_expires_at <= $2)
	`, id, currentTime.UTC())
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		return nil
	}

	job, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrJobNotFound) {
			return ErrJobNotFound
		}
		return fmt.Errorf("failed to re-check job after delete attempt: %w", err)
	}

	if !isJobStatusDeletable(job.Status) {
		return ErrJobNotDeletable
	}

	if job.LeaseExpiresAt != nil && currentTime.Before(*job.LeaseExpiresAt) {
		return ErrJobReserved
	}

	return errors.New("unexpected state: job is in deletable state but delete failed")
}

// DeleteByPayloadField deletes jobs by matching a JSON field in the payload.
func (r *JobRepo) DeleteByPayloadField(ctx context.Context, params core.DeleteByPayloadFieldParams) (int, error) {
	if !params.JobType.Valid() {
		return 0, fmt.Errorf("invalid job type: %s", params.JobType)
	}

	currentTime := r.timeProvider.Now()
	res, err := r.DB.ExecContext(ctx, `
		DELETE FROM jobs
		WHERE type = $1
		  AND status = 'pending'
		  AND (lease_expires_at IS NULL OR lease_expires_at <= $2)
		  AND payload->$3 = to_jsonb($4::text)
	`, params.JobType, currentTime.UTC(), params.FieldName, params.FieldValue)
	if err != nil {
		return 0, fmt.Errorf("delete jobs by payload field: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// isJobStatusDeletable returns true if a job in the given status can be safely deleted.
func isJobStatusDeletable(status model.JobStatus) bool {
	return status == model.JobStatusPending ||
		status == model.JobStatusCompleted ||
		status == model.JobStatusFailed
}
