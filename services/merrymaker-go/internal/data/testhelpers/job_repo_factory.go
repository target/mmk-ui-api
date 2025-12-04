package testhelpers

import (
	"database/sql"

	"github.com/target/mmk-ui-api/internal/data"
)

// NewJobRepoWithTimeProvider creates a JobRepo with the provided TimeProvider for tests.
func NewJobRepoWithTimeProvider(db *sql.DB, cfg data.RepoConfig, tp data.TimeProvider) *data.JobRepo {
	cfg.TimeProvider = tp
	return data.NewJobRepo(db, cfg)
}
