// Package httpx provides HTTP handlers and utilities for the merrymaker job system API.
package httpx

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

// JobHandlers provides HTTP handlers for job-related operations.
type JobHandlers struct {
	Svc          *service.JobService
	Orchestrator *service.RulesOrchestrationService
}

// CreateJob handles HTTP requests to create a new job.
func (h *JobHandlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req core.CreateJobRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	job, err := h.Svc.Create(r.Context(), &req)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "create_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, job)
}

const (
	defaultLeaseSeconds = 30
)

// ReserveNext handles HTTP requests to reserve the next available job.
func (h *JobHandlers) ReserveNext(w http.ResponseWriter, r *http.Request) {
	jobType := core.JobType(r.PathValue("type"))
	if jobType == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job type is required")},
		)
		return
	}
	lease := parseIntQuery(r, "lease", defaultLeaseSeconds)
	wait := parseIntQuery(r, "wait", 0)

	// First attempt
	if job, err := h.tryReserveJob(r.Context(), jobType, lease); err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "reserve_failed", Err: err})
		return
	} else if job != nil {
		WriteJSON(w, http.StatusOK, job)
		return
	}

	if wait <= 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.handleLongPoll(w, r, longPollParams{
		jobType: jobType,
		lease:   lease,
		wait:    wait,
	})
}

func (h *JobHandlers) tryReserveJob(
	ctx context.Context,
	jobType core.JobType,
	lease int,
) (*model.Job, error) {
	job, err := h.Svc.ReserveNext(ctx, jobType, time.Duration(lease)*time.Second)
	if err != nil && !errors.Is(err, model.ErrNoJobsAvailable) {
		return nil, err
	}
	return job, nil
}

type longPollParams struct {
	jobType core.JobType
	lease   int
	wait    int
}

func (h *JobHandlers) handleLongPoll(w http.ResponseWriter, r *http.Request, params longPollParams) {
	dur := time.Duration(params.wait) * time.Second
	if dur <= 0 {
		dur = time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), dur)
	defer cancel()

	unsub, ch := h.Svc.Subscribe(params.jobType)
	defer unsub()

	for {
		select {
		case <-ctx.Done():
			w.WriteHeader(http.StatusNoContent)
			return
		case <-ch:
			if job, err := h.tryReserveJob(ctx, params.jobType, params.lease); err != nil {
				WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "reserve_failed", Err: err})
				return
			} else if job != nil {
				WriteJSON(w, http.StatusOK, job)
				return
			}
			// No job yet; keep waiting until ctx timeout to handle missed/duplicate signals.
		}
	}
}

// Heartbeat handles HTTP requests to extend a job lease.
func (h *JobHandlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job id is required")},
		)
		return
	}
	extend := parseIntQuery(r, "extend", defaultLeaseSeconds)

	success, err := h.Svc.Heartbeat(r.Context(), jobID, time.Duration(extend)*time.Second)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "heartbeat_failed", Err: err})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": success})
}

// Complete handles HTTP requests to mark a job as completed.
func (h *JobHandlers) Complete(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job id is required")},
		)
		return
	}

	success, err := h.Svc.Complete(r.Context(), jobID)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "complete_failed", Err: err})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": success})
}

// Fail handles HTTP requests to mark a job as failed with an error message.
func (h *JobHandlers) Fail(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job id is required")},
		)
		return
	}

	var body struct {
		Error string `json:"error"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	success, err := h.Svc.Fail(r.Context(), jobID, body.Error)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "fail_failed", Err: err})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": success})
}

// Stats handles HTTP requests to retrieve job stats for a job type.
func (h *JobHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	jobType := core.JobType(r.PathValue("type"))
	if jobType == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job type is required")},
		)
		return
	}

	stats, err := h.Svc.Stats(r.Context(), jobType)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "stats_failed", Err: err})
		return
	}
	WriteJSON(w, http.StatusOK, stats)
}

// GetStatus handles HTTP requests to retrieve the status of a specific job.
func (h *JobHandlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job id is required")},
		)
		return
	}

	status, err := h.Svc.GetStatus(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, data.ErrJobNotFound) {
			WriteError(
				w,
				ErrorParams{Code: http.StatusNotFound, ErrCode: "job_not_found", Err: errors.New("job not found")},
			)
		} else {
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "get_status_failed", Err: errors.New("failed to get job status")})
		}
		return
	}
	WriteJSON(w, http.StatusOK, status)
}

// GetRulesResults returns cached rules processing results for a given rules job, if available.
func (h *JobHandlers) GetRulesResults(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job id is required")},
		)
		return
	}
	if h.Orchestrator == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	res, err := h.Orchestrator.GetJobResults(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, service.ErrRulesResultsNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "get_rules_results_failed", Err: err})
		return
	}
	WriteJSON(w, http.StatusOK, res)
}
