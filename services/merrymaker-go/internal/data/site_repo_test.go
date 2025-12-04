package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func createTestSource(t *testing.T, db *sql.DB, name string) *model.Source {
	t.Helper()
	sr := NewSourceRepo(db)
	s, err := sr.Create(context.Background(), &model.CreateSourceRequest{
		Name:  name,
		Value: "val",
		Test:  false,
	})
	require.NoError(t, err)
	return s
}

func TestSiteRepo_Create_Get_List_Update_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		repo := NewSiteRepo(db)

		// prerequisites
		src := createTestSource(t, db, fmt.Sprintf("src-%d", time.Now().UnixNano()))

		// create
		req := &model.CreateSiteRequest{
			Name:            fmt.Sprintf("site-%d", time.Now().UnixNano()),
			Enabled:         nil, // default true
			Scope:           testutil.StringPtr("*"),
			HTTPAlertSinkID: nil,
			RunEveryMinutes: 15,
			SourceID:        src.ID,
		}
		s, err := repo.Create(ctx, req)
		require.NoError(t, err)
		require.NotEmpty(t, s.ID)
		assert.True(t, s.Enabled)
		assert.Equal(t, model.SiteAlertModeActive, s.AlertMode)
		assert.NotZero(t, s.CreatedAt)

		// get by id
		got, err := repo.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, s.Name, got.Name)

		// get by name
		byName, err := repo.GetByName(ctx, s.Name)
		require.NoError(t, err)
		assert.Equal(t, s.ID, byName.ID)

		// list
		lst, err := repo.List(ctx, 10, 0)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(lst), 1)

		// update - disable, change scope, run_every, reassign source
		newScope := "prod"
		newEvery := 30
		muted := model.SiteAlertModeMuted
		upd := model.UpdateSiteRequest{
			Enabled:         testutil.BoolPtr(false),
			Scope:           &newScope,
			RunEveryMinutes: &newEvery,
			AlertMode:       &muted,
		}
		updated, err := repo.Update(ctx, s.ID, upd)
		require.NoError(t, err)
		assert.False(t, updated.Enabled)
		assert.Equal(t, model.SiteAlertModeMuted, updated.AlertMode)
		assert.Equal(t, newScope, *updated.Scope)
		assert.Equal(t, newEvery, updated.RunEveryMinutes)

		// update - enable should set last_enabled
		en := true
		updated2, err := repo.Update(ctx, s.ID, model.UpdateSiteRequest{Enabled: &en})
		require.NoError(t, err)
		assert.True(t, updated2.Enabled)
		assert.Equal(t, model.SiteAlertModeMuted, updated2.AlertMode)
		if assert.NotNil(t, updated2.LastEnabled) {
			assert.WithinDuration(t, time.Now(), *updated2.LastEnabled, 5*time.Second)
		}

		// delete
		deleted, err := repo.Delete(ctx, s.ID)
		require.NoError(t, err)
		assert.True(t, deleted)

		_, err = repo.GetByID(ctx, s.ID)
		require.Error(t, err)
	})
}

func TestSiteRepo_DuplicateName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSiteRepo(db)
		ctx := context.Background()
		src := createTestSource(t, db, fmt.Sprintf("src-%d", time.Now().UnixNano()))

		name := fmt.Sprintf("dup-site-%d", time.Now().UnixNano())
		_, err := repo.Create(ctx, &model.CreateSiteRequest{
			Name:            name,
			RunEveryMinutes: 5,
			SourceID:        src.ID,
		})
		require.NoError(t, err)

		_, err = repo.Create(ctx, &model.CreateSiteRequest{
			Name:            name,
			RunEveryMinutes: 10,
			SourceID:        src.ID,
		})
		require.Error(t, err)
	})
}

func TestSiteRepo_Create_ValidationErrors(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSiteRepo(db)
		ctx := context.Background()

		// empty name
		_, err := repo.Create(ctx, &model.CreateSiteRequest{
			Name:            " ",
			RunEveryMinutes: 5,
			SourceID:        "src",
		})
		require.Error(t, err)

		// too long name (>255)
		longName := strings.Repeat("a", 256)
		_, err = repo.Create(ctx, &model.CreateSiteRequest{
			Name:            longName,
			RunEveryMinutes: 5,
			SourceID:        "src",
		})
		require.Error(t, err)

		// invalid run_every_minutes
		_, err = repo.Create(ctx, &model.CreateSiteRequest{
			Name:            "ok",
			RunEveryMinutes: 0,
			SourceID:        "src",
		})
		require.Error(t, err)

		// missing source_id
		_, err = repo.Create(ctx, &model.CreateSiteRequest{
			Name:            "ok",
			RunEveryMinutes: 1,
			SourceID:        " ",
		})
		require.Error(t, err)

		_, err = repo.Create(ctx, &model.CreateSiteRequest{
			Name:            "ok",
			RunEveryMinutes: 5,
			SourceID:        "src",
			AlertMode:       model.SiteAlertMode("invalid"),
		})
		require.Error(t, err)
	})
}

func TestSiteRepo_Update_ValidationErrors(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSiteRepo(db)
		ctx := context.Background()

		// create a valid site first
		src := createTestSource(t, db, fmt.Sprintf("src-%d", time.Now().UnixNano()))
		s, err := repo.Create(ctx, &model.CreateSiteRequest{
			Name:            fmt.Sprintf("site-%d", time.Now().UnixNano()),
			RunEveryMinutes: 5,
			SourceID:        src.ID,
		})
		require.NoError(t, err)

		// empty update
		_, err = repo.Update(ctx, s.ID, model.UpdateSiteRequest{})
		require.Error(t, err)

		// invalid name
		empty := " "
		_, err = repo.Update(ctx, s.ID, model.UpdateSiteRequest{Name: &empty})
		require.Error(t, err)

		// too long name
		tooLong := strings.Repeat("x", 256)
		_, err = repo.Update(ctx, s.ID, model.UpdateSiteRequest{Name: &tooLong})
		require.Error(t, err)

		// invalid run_every_minutes
		zero := 0
		_, err = repo.Update(ctx, s.ID, model.UpdateSiteRequest{RunEveryMinutes: &zero})
		require.Error(t, err)

		// empty source id
		blank := ""
		_, err = repo.Update(ctx, s.ID, model.UpdateSiteRequest{SourceID: &blank})
		require.Error(t, err)

		badMode := model.SiteAlertMode("wrong")
		_, err = repo.Update(ctx, s.ID, model.UpdateSiteRequest{AlertMode: &badMode})
		require.Error(t, err)
	})
}
