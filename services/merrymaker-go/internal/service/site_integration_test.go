package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func TestSiteService_CreateAndUpdate_ReconcilesSchedule(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		// repos and service
		sitesRepo := data.NewSiteRepo(db)
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		svc := NewSiteService(SiteServiceOptions{SiteRepo: sitesRepo, Admin: adminRepo})

		// Create a source (required by Site)
		source, err := data.NewSourceRepo(db).Create(ctx, &model.CreateSourceRequest{
			Name:  fmt.Sprintf("src-%d", time.Now().UnixNano()),
			Value: "https://example.com",
			Test:  true,
		})
		require.NoError(t, err)

		// Create an enabled site
		enabled := true
		site, err := svc.Create(ctx, &model.CreateSiteRequest{
			Name:            fmt.Sprintf("site-%d", time.Now().UnixNano()),
			Enabled:         &enabled,
			RunEveryMinutes: 2,
			SourceID:        source.ID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, site.ID)

		// Sanity: fetch by ID exists
		got, err := data.NewSiteRepo(db).GetByID(ctx, site.ID)
		require.NoError(t, err)
		require.Equal(t, site.ID, got.ID)

		// Verify scheduled_jobs row exists with expected interval seconds
		assertScheduleIntervalSeconds(t, db, taskNameForSite(site.ID), 120)

		// Update interval
		newEvery := 1
		updated, err := svc.Update(ctx, site.ID, model.UpdateSiteRequest{RunEveryMinutes: &newEvery})
		require.NoError(t, err)
		assert.Equal(t, 1, updated.RunEveryMinutes)
		assertScheduleIntervalSeconds(t, db, taskNameForSite(site.ID), 60)

		// Disable -> schedule removed
		disabled := false
		updated, err = svc.Update(ctx, site.ID, model.UpdateSiteRequest{Enabled: &disabled})
		require.NoError(t, err)
		assert.False(t, updated.Enabled)
		assertNoSchedule(t, db, taskNameForSite(site.ID))
	})
}

func TestSiteService_Delete_RemovesSchedule(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		sitesRepo := data.NewSiteRepo(db)
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		svc := NewSiteService(SiteServiceOptions{SiteRepo: sitesRepo, Admin: adminRepo})

		source, err := data.NewSourceRepo(db).Create(ctx, &model.CreateSourceRequest{
			Name:  fmt.Sprintf("src-%d", time.Now().UnixNano()),
			Value: "https://example.com",
			Test:  true,
		})
		require.NoError(t, err)

		enabled := true
		site, err := svc.Create(ctx, &model.CreateSiteRequest{
			Name:            fmt.Sprintf("site-%d", time.Now().UnixNano()),
			Enabled:         &enabled,
			RunEveryMinutes: 5,
			SourceID:        source.ID,
		})
		require.NoError(t, err)
		assertScheduleIntervalSeconds(t, db, taskNameForSite(site.ID), 300)

		ok, err := svc.Delete(ctx, site.ID)
		require.NoError(t, err)
		assert.True(t, ok)
		assertNoSchedule(t, db, taskNameForSite(site.ID))
	})
}

// Helpers.
func assertScheduleIntervalSeconds(t *testing.T, db *sql.DB, taskName string, wantSeconds int64) {
	t.Helper()
	var got int64
	err := db.QueryRow(`SELECT EXTRACT(EPOCH FROM scheduled_interval)::bigint FROM scheduled_jobs WHERE task_name = $1`, taskName).
		Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, wantSeconds, got)
}

func assertNoSchedule(t *testing.T, db *sql.DB, taskName string) {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM scheduled_jobs WHERE task_name = $1`, taskName).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
