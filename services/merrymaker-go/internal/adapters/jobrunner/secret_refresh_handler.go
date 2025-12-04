package jobrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// handleSecretRefreshJob processes a secret refresh job by executing the provider script.
func (r *Runner) handleSecretRefreshJob(ctx context.Context, job *model.Job) error {
	if r.secretRefreshSvc == nil {
		return errors.New("secret refresh service not configured")
	}

	// Decode job payload
	var p struct {
		SecretID string `json:"secret_id"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if p.SecretID == "" {
		return errors.New("missing secret_id in job payload")
	}

	// Execute secret refresh
	if err := r.secretRefreshSvc.ExecuteRefresh(ctx, p.SecretID); err != nil {
		return fmt.Errorf("execute secret refresh: %w", err)
	}

	return nil
}
