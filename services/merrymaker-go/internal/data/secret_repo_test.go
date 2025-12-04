package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey() []byte {
	// Derive a deterministic 32-byte key from a phrase for tests
	sum := sha256.Sum256([]byte("merrymaker-test-key"))
	return sum[:]
}

func newTestSecretRepo(t *testing.T, db *sql.DB) *SecretRepo {
	enc, err := cryptoutil.NewAESGCMEncryptor(testKey())
	require.NoError(t, err)
	return NewSecretRepo(db, enc)
}

func TestSecretRepo_Create_GetByName_RoundTrip(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := newTestSecretRepo(t, db)
		ctx := context.Background()

		plain := "super-secret-value"
		created, err := repo.Create(ctx, model.CreateSecretRequest{
			Name:  "API_TOKEN",
			Value: plain,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "API_TOKEN", created.Name)
		assert.Equal(t, plain, created.Value)

		// Ensure stored in DB as encrypted (not plaintext)
		var stored string
		require.NoError(t, db.QueryRow(`SELECT value FROM secrets WHERE id = $1`, created.ID).Scan(&stored))
		assert.NotEqual(t, plain, stored)
		assert.True(t, strings.HasPrefix(stored, "v1:"))

		// Get by name decrypts
		fetched, err := repo.GetByName(ctx, "API_TOKEN")
		require.NoError(t, err)
		assert.Equal(t, created.ID, fetched.ID)
		assert.Equal(t, plain, fetched.Value)
	})
}

func TestSecretRepo_List_NoValues(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := newTestSecretRepo(t, db)
		ctx := context.Background()

		_, err := repo.Create(ctx, model.CreateSecretRequest{Name: "S1", Value: "v1"})
		require.NoError(t, err)
		_, err = repo.Create(ctx, model.CreateSecretRequest{Name: "S2", Value: "v2"})
		require.NoError(t, err)

		list, err := repo.List(ctx, 10, 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(list), 2)
		for _, s := range list {
			assert.Empty(t, s.Value)
		}
	})
}

func TestSecretRepo_Update_And_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := newTestSecretRepo(t, db)
		ctx := context.Background()

		created, err := repo.Create(ctx, model.CreateSecretRequest{Name: "UPD", Value: "old"})
		require.NoError(t, err)

		newName := "UPD2"
		newVal := "new-value"
		updated, err := repo.Update(ctx, created.ID, model.UpdateSecretRequest{
			Name:  &newName,
			Value: &newVal,
		})
		require.NoError(t, err)
		assert.Equal(t, newName, updated.Name)
		assert.Equal(t, newVal, updated.Value)

		// Raw DB should not have plaintext
		var stored string
		require.NoError(t, db.QueryRow(`SELECT value FROM secrets WHERE id = $1`, created.ID).Scan(&stored))
		assert.NotEqual(t, newVal, stored)

		// Delete
		ok, err := repo.Delete(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, ok)

		// Should not find afterwards
		_, err = repo.GetByID(ctx, created.ID)
		require.Error(t, err)
	})
}

func TestSecretRepo_Constraints(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := newTestSecretRepo(t, db)
		ctx := context.Background()

		_, err := repo.Create(ctx, model.CreateSecretRequest{Name: "DUP", Value: "a"})
		require.NoError(t, err)
		_, err = repo.Create(ctx, model.CreateSecretRequest{Name: "DUP", Value: "b"})
		require.Error(t, err)
		require.ErrorContains(t, err, "already exists")

		// Invalid updates
		_, err = repo.Update(ctx, "00000000-0000-0000-0000-000000000000", model.UpdateSecretRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one field")
	})
}

func TestSecretRepo_DecryptFailure(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Use a different key to create and then attempt to decrypt with wrong key to simulate failure
		enc1, _ := cryptoutil.NewAESGCMEncryptor(testKey())
		enc2, _ := cryptoutil.NewAESGCMEncryptor([]byte(hex.EncodeToString(testKey()))[:32])

		repo1 := NewSecretRepo(db, enc1)
		repo2 := NewSecretRepo(db, enc2)

		ctx := context.Background()
		created, err := repo1.Create(ctx, model.CreateSecretRequest{Name: "KEY1", Value: "vv"})
		require.NoError(t, err)

		_, err = repo2.GetByID(ctx, created.ID)
		require.Error(t, err)
	})
}
