package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	// Import pgx driver for database/sql compatibility in tests.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/target/mmk-ui-api/internal/migrate"
)

// RunMigrations delegates to the shared migrate package to apply production migrations.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	return migrate.Run(ctx, db)
}

// TestDBConfig holds configuration for test database.
type TestDBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// DefaultTestDBConfig returns default test database configuration.
// Defaults to port 55432 (local test DB from docker-compose test profile).
// CI/CD environments should set TEST_DB_PORT=5432 explicitly.
func DefaultTestDBConfig() TestDBConfig {
	return TestDBConfig{
		Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:     getEnvOrDefault("TEST_DB_PORT", "55432"),
		User:     getEnvOrDefault("TEST_DB_USER", "merrymaker"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "merrymaker"),
		DBName:   getEnvOrDefault("TEST_DB_NAME", "merrymaker"),
	}
}

// SetupTestDB creates a test database connection and runs migrations.
func SetupTestDB(t TestingTB) *sql.DB {
	t.Helper()
	SkipIfNoTestDB(t)

	cfg := DefaultTestDBConfig()
	hostPort := net.JoinHostPort(cfg.Host, cfg.Port)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		cfg.User, cfg.Password, hostPort, cfg.DBName)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("Failed to open database:", err)
	}

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		t.Fatal("Failed to connect to test database. Make sure PostgreSQL is running (docker-compose up -d):", err)
	}

	// Run production migrations to ensure schema matches actual application
	if migrateErr := RunMigrations(ctx, db); migrateErr != nil {
		t.Fatal("Failed to run migrations:", migrateErr)
	}

	// Clean up any existing test data
	CleanupTestDB(t, db)

	return db
}

// CleanupTestDB removes all test data from the database.
func CleanupTestDB(t TestingTB, db *sql.DB) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete in reverse dependency order - using explicit queries for security
	// Respect FKs: association tables reference main tables + secrets; delete them first.
	if _, err := db.ExecContext(ctx, "DELETE FROM source_secrets"); err != nil {
		t.Fatalf("Failed to clean up table source_secrets: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM http_alert_sink_secrets"); err != nil {
		t.Fatalf("Failed to clean up table http_alert_sink_secrets: %v", err)
	}
	// Clean up sites (depends on sources and http_alert_sinks); must delete before deleting those parents
	if _, err := db.ExecContext(ctx, "DELETE FROM sites"); err != nil {
		t.Fatalf("Failed to clean up table sites: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM jobs"); err != nil {
		t.Fatalf("Failed to clean up table jobs: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM events"); err != nil {
		t.Fatalf("Failed to clean up table events: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM sources"); err != nil {
		t.Fatalf("Failed to clean up table sources: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM http_alert_sinks"); err != nil {
		t.Fatalf("Failed to clean up table http_alert_sinks: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM secrets"); err != nil {
		t.Fatalf("Failed to clean up table secrets: %v", err)
	}
}

// TeardownTestDB closes the database connection.
func TeardownTestDB(t TestingTB, db *sql.DB) {
	t.Helper()
	if db != nil {
		CleanupTestDB(t, db)
		err := db.Close()
		if err != nil {
			t.Fatal("Failed to close database:", err)
		}
	}
}

// WithTestDB is a helper that sets up and tears down a test database.
func WithTestDB(t TestingTB, fn func(*sql.DB)) {
	t.Helper()
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)
	fn(db)
}

// SetupAutoDB chooses ephemeral per-test schema when TEST_DB_EPHEMERAL is truthy, otherwise shared test DB.
func SetupAutoDB(t TestingTB) *sql.DB {
	t.Helper()
	SkipIfNoTestDB(t)
	if envBool("TEST_DB_EPHEMERAL") {
		return SetupEphemeralSchemaDB(t)
	}
	return SetupTestDB(t)
}

// WithAutoDB wraps SetupAutoDB and tears down when using shared DB mode.
// For ephemeral mode, schema cleanup is handled by SetupEphemeralSchemaDB via t.Cleanup.
func WithAutoDB(t TestingTB, fn func(*sql.DB)) {
	t.Helper()
	if envBool("TEST_DB_EPHEMERAL") {
		db := SetupEphemeralSchemaDB(t)
		fn(db)
		return
	}
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)
	fn(db)
}

// TestingTB is an interface that covers both *testing.T and *testing.B.
type TestingTB interface {
	Helper()
	Skip(args ...interface{})
	Skipf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

// SkipIfNoTestDB skips the test if test database is not available.
func SkipIfNoTestDB(t TestingTB) {
	t.Helper()

	cfg := DefaultTestDBConfig()
	hostPort := net.JoinHostPort(cfg.Host, cfg.Port)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		cfg.User, cfg.Password, hostPort, cfg.DBName)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		if requireDB() {
			t.Fatal("Test database not available:", err)
		}
		t.Skip("Test database not available:", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			t.Logf("test db close failed: %v", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if pingErr := db.PingContext(ctx); pingErr != nil {
		if requireDB() {
			t.Fatal("Test database not available:", pingErr)
		}
		t.Skip("Test database not available:", pingErr)
	}
}

// buildBaseDSN constructs a base DSN without search_path.
func buildBaseDSN(cfg TestDBConfig) string {
	hostPort := net.JoinHostPort(cfg.Host, cfg.Port)
	sslMode := getEnvOrDefault("DB_SSL_MODE", "disable")
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=%s",
		cfg.User, cfg.Password, hostPort, cfg.DBName, sslMode,
	)
}

// generateSchemaName creates a lowercase alphanumeric schema name with prefix.
func generateSchemaName() string {
	// 4 random bytes -> 8 hex chars
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based suffix if randomness fails
		return fmt.Sprintf("t_%d", time.Now().UnixNano())
	}
	return "t_" + hex.EncodeToString(b)
}

func closeAndLog(t TestingTB, name string, closer interface{ Close() error }) {
	if err := closer.Close(); err != nil {
		t.Logf("warning: failed to close %s: %v", name, err)
	}
}

// SetupEphemeralSchemaDB creates a unique schema per test, sets search_path to it, runs migrations,
// and registers cleanup to drop the schema after the test completes.
func SetupEphemeralSchemaDB(t TestingTB) *sql.DB {
	t.Helper()
	SkipIfNoTestDB(t)

	adminDB := openAdminDB(t)
	schema := createSchema(t, adminDB)
	db := openDBWithSchema(t, adminDB, schema)
	// Register cleanup before running migrations to ensure resources are released even if migrations fail.
	registerSchemaCleanup(t, schemaCleanupResources{
		adminDB: adminDB,
		db:      db,
		schema:  schema,
	})
	migrateSchema(t, db)
	return db
}

func openAdminDB(t TestingTB) *sql.DB {
	cfg := DefaultTestDBConfig()
	baseDSN := buildBaseDSN(cfg)
	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatal("Failed to open admin DB:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if pingErr := adminDB.PingContext(ctx); pingErr != nil {
		closeAndLog(t, "admin DB", adminDB)
		t.Fatal("Failed to ping admin DB:", pingErr)
	}
	return adminDB
}

func createSchema(t TestingTB, adminDB *sql.DB) string {
	schema := generateSchemaName()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		closeAndLog(t, "admin DB", adminDB)
		t.Fatalf("Failed to create schema %s: %v", schema, err)
	}
	return schema
}

func openDBWithSchema(t TestingTB, adminDB *sql.DB, schema string) *sql.DB {
	cfg := DefaultTestDBConfig()
	baseDSN := buildBaseDSN(cfg)
	u, err := url.Parse(baseDSN)
	if err != nil {
		closeAndLog(t, "admin DB", adminDB)
		t.Fatal("Failed to parse DSN:", err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	dsnWithSchema := u.String()

	db, err := sql.Open("pgx", dsnWithSchema)
	if err != nil {
		closeAndLog(t, "admin DB", adminDB)
		t.Fatal("Failed to open schema-scoped DB:", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if pingErr := db.PingContext(ctx); pingErr != nil {
		closeAndLog(t, "schema DB", db)
		closeAndLog(t, "admin DB", adminDB)
		t.Fatal("Failed to ping schema-scoped DB:", pingErr)
	}
	return db
}

func migrateSchema(t TestingTB, db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if migrateErr := RunMigrations(ctx, db); migrateErr != nil {
		closeAndLog(t, "schema DB", db)
		// Best-effort close of admin handle is handled by the cleanup we register; nothing to do here.
		t.Fatal("Failed to run migrations in ephemeral schema:", migrateErr)
	}
}

type schemaCleanupResources struct {
	adminDB *sql.DB
	db      *sql.DB
	schema  string
}

func registerSchemaCleanup(t TestingTB, resources schemaCleanupResources) {
	t.Logf("Using ephemeral schema: %s", resources.schema)
	tCleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		closeAndLog(t, "schema DB", resources.db)
		if _, err := resources.adminDB.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+resources.schema+" CASCADE"); err != nil {
			t.Logf("Warning: failed to drop schema %s: %v", resources.schema, err)
		}
		closeAndLog(t, "admin DB", resources.adminDB)
	}
	if tc, ok := any(t).(interface{ Cleanup(func()) }); ok {
		tc.Cleanup(tCleanup)
	} else {
		defer tCleanup()
	}
}

// WithEphemeralDB wraps SetupEphemeralSchemaDB and runs the provided function.
func WithEphemeralDB(t TestingTB, fn func(*sql.DB)) {
	t.Helper()
	db := SetupEphemeralSchemaDB(t)
	fn(db)
}

// getEnvOrDefault returns environment variable value or default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// envBool parses common truthy values from env vars.
func envBool(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func requireDB() bool    { return envBool("TEST_REQUIRE_DB") || envBool("TEST_REQUIRE_INFRA") }
func requireRedis() bool { return envBool("TEST_REQUIRE_REDIS") || envBool("TEST_REQUIRE_INFRA") }

// FixedTimeFunc returns a function that always returns the same time.
func FixedTimeFunc(t time.Time) func() time.Time {
	return func() time.Time {
		return t
	}
}

// TestTime returns a fixed time for testing.
func TestTime() time.Time {
	return time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
}

// JobStateInfo represents the state of a job for debugging.
type JobStateInfo struct {
	ID          string     `db:"id"`
	Type        string     `db:"type"`
	Status      string     `db:"status"`
	RetryCount  int        `db:"retry_count"`
	MaxRetries  int        `db:"max_retries"`
	LastError   *string    `db:"last_error"`
	CompletedAt *time.Time `db:"completed_at"`
}

// InspectJobStates returns information about all jobs in the database for debugging.
func InspectJobStates(t TestingTB, db *sql.DB) []JobStateInfo {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT id, type, status, retry_count, max_retries, last_error, completed_at
		FROM jobs
		ORDER BY created_at ASC
	`)
	if err != nil {
		t.Fatalf("Failed to query job states: %v", err)
	}
	defer func() {
		if rerr := rows.Close(); rerr != nil {
			t.Logf("warning: failed to close job state rows: %v", rerr)
		}
	}()

	var jobs []JobStateInfo
	for rows.Next() {
		var job JobStateInfo
		scanErr := rows.Scan(
			&job.ID,
			&job.Type,
			&job.Status,
			&job.RetryCount,
			&job.MaxRetries,
			&job.LastError,
			&job.CompletedAt,
		)
		if scanErr != nil {
			t.Fatalf("Failed to scan job state: %v", scanErr)
		}
		jobs = append(jobs, job)
	}

	if iterErr := rows.Err(); iterErr != nil {
		t.Fatalf("Error iterating over rows: %v", iterErr)
	}

	return jobs
}

// LogJobStates logs the current state of all jobs for debugging.
func LogJobStates(t TestingTB, db *sql.DB, message string) {
	t.Helper()

	jobs := InspectJobStates(t, db)
	t.Logf("=== %s ===", message)
	for i, job := range jobs {
		t.Logf("Job %d: ID=%s, Status=%s, RetryCount=%d/%d, LastError=%v, CompletedAt=%v",
			i+1, job.ID[:8], job.Status, job.RetryCount, job.MaxRetries, job.LastError, job.CompletedAt)
	}
	t.Logf("=== End %s ===", message)
}

// ConcurrentTestRunner provides utilities for testing concurrent operations.
type ConcurrentTestRunner struct {
	t  TestingTB
	db *sql.DB
}

// NewConcurrentTestRunner creates a new concurrent test runner.
func NewConcurrentTestRunner(t TestingTB, db *sql.DB) *ConcurrentTestRunner {
	return &ConcurrentTestRunner{t: t, db: db}
}

// RunConcurrent runs multiple functions concurrently and waits for all to complete.
func (r *ConcurrentTestRunner) RunConcurrent(funcs ...func() error) []error {
	r.t.Helper()

	results := make(chan error, len(funcs))

	for _, f := range funcs {
		go func(fn func() error) {
			results <- fn()
		}(f)
	}

	errors := make([]error, len(funcs))
	for i := range funcs {
		errors[i] = <-results
	}

	return errors
}

// AssertNoErrors checks that none of the errors are non-nil.
func (r *ConcurrentTestRunner) AssertNoErrors(errors []error) {
	r.t.Helper()

	for i, err := range errors {
		if err != nil {
			r.t.Fatalf("Concurrent operation %d failed: %v", i, err)
		}
	}
}

// TestTimeProvider provides a simple time provider for testing.
type TestTimeProvider struct {
	currentTime time.Time
}

// NewTestTimeProvider creates a new test time provider.
func NewTestTimeProvider(startTime time.Time) *TestTimeProvider {
	return &TestTimeProvider{currentTime: startTime}
}

// Now returns the current time.
func (p *TestTimeProvider) Now() time.Time {
	return p.currentTime
}

// SetTime sets the current time.
func (p *TestTimeProvider) SetTime(t time.Time) {
	p.currentTime = t
}

// AddTime advances the current time by the given duration.
func (p *TestTimeProvider) AddTime(d time.Duration) {
	p.currentTime = p.currentTime.Add(d)
}

// TimeBasedTestHelper provides utilities for testing time-based operations.
type TimeBasedTestHelper struct {
	t            TestingTB
	db           *sql.DB
	timeProvider *TestTimeProvider
}

// NewTimeBasedTestHelper creates a new time-based test helper.
func NewTimeBasedTestHelper(t TestingTB, db *sql.DB, startTime time.Time) *TimeBasedTestHelper {
	return &TimeBasedTestHelper{
		t:            t,
		db:           db,
		timeProvider: NewTestTimeProvider(startTime),
	}
}

// GetTimeProvider returns the time provider for use in repositories.
func (h *TimeBasedTestHelper) GetTimeProvider() *TestTimeProvider {
	return h.timeProvider
}

// AdvanceTime advances the time by the given duration.
func (h *TimeBasedTestHelper) AdvanceTime(d time.Duration) {
	h.timeProvider.AddTime(d)
	h.t.Logf("Advanced time by %v to %v", d, h.timeProvider.Now())
}

// SetTime sets the time to a specific value.
func (h *TimeBasedTestHelper) SetTime(t time.Time) {
	h.timeProvider.SetTime(t)
	h.t.Logf("Set time to %v", t)
}

// WaitForCondition polls a condition function until it returns true or timeout is reached.
func (h *TimeBasedTestHelper) WaitForCondition(
	condition func() bool,
	timeout time.Duration,
	pollInterval time.Duration,
) bool {
	h.t.Helper()

	start := h.timeProvider.Now()
	for h.timeProvider.Now().Sub(start) < timeout {
		if condition() {
			return true
		}
		h.AdvanceTime(pollInterval)
	}
	return false
}

// Redis test utilities

// GetTestRedisAddr returns the appropriate Redis address for testing.
// It checks environment variables to determine if we're in CI or local development.
// Returns the address and whether Redis is available at that address.
func GetTestRedisAddr(t TestingTB) (string, bool) {
	t.Helper()

	// Check for CI environment variable (Vela CI sets REDIS_ADDR)
	if ciAddr := os.Getenv("REDIS_ADDR"); ciAddr != "" {
		return testRedisConnection(t, ciAddr)
	}

	// Try common CI Redis addresses
	ciAddresses := []string{
		"redis:6379",     // Docker Compose service name in CI
		"localhost:6379", // Alternative CI setup
	}

	for _, candidate := range ciAddresses {
		if validatedAddr, ok := testRedisConnection(t, candidate); ok {
			return validatedAddr, true
		}
	}

	// Default to local test Redis address
	return testRedisConnection(t, "localhost:56379")
}

// testRedisConnection tests if Redis is available at the given address.
func testRedisConnection(t TestingTB, addr string) (string, bool) {
	t.Helper()

	client := redis.NewClient(&redis.Options{Addr: addr})
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("warning: failed to close redis client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Logf("Redis not available at %s: %v", addr, err)
		return addr, false
	}

	return addr, true
}

// selectTestRedisDB chooses a Redis DB index for tests to avoid cross-package interference.
// Priority:
//  1. TEST_REDIS_DB env var if set and valid (>=0)
//  2. Reserve a DB in [1..15] by acquiring a lock key in a meta DB (DB 0) so FlushDB
//     in the selected test DB won't remove the reservation
//  3. Fallback to DB=1.
func selectTestRedisDB(t TestingTB, addr string) int {
	// Allow explicit override
	if v := os.Getenv("TEST_REDIS_DB"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= 0 {
			return i
		}
		t.Logf("Invalid TEST_REDIS_DB=%q, falling back to auto-select", v)
	}

	// Use a meta DB (DB 0) for reservation keys to avoid being wiped by FlushDB on test DBs
	meta := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
	defer func() {
		if err := meta.Close(); err != nil {
			t.Logf("warning: failed to close redis meta client: %v", err)
		}
	}()

	for i := 1; i <= 15; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		lockKey := fmt.Sprintf("merrymaker:testutil:db_lock:%d", i)
		lockVal := fmt.Sprintf("%d:%d", os.Getpid(), time.Now().UnixNano())
		ok, err := meta.SetNX(ctx, lockKey, lockVal, 30*time.Minute).Result()
		cancel()
		if err != nil || !ok {
			continue
		}

		registerRedisCleanup(t, addr, lockKey)
		t.Logf("Using Redis DB=%d for tests at %s", i, addr)
		return i
	}

	t.Logf("Falling back to Redis DB=1 for tests at %s", addr)
	return 1
}

func registerRedisCleanup(t TestingTB, addr, lockKey string) {
	tc, ok := any(t).(interface{ Cleanup(func()) })
	if !ok {
		return
	}

	tc.Cleanup(func() {
		c := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		if err := c.Del(ctx2, lockKey).Err(); err != nil {
			t.Logf("warning: failed to release redis db lock %s: %v", lockKey, err)
		}
		cancel2()
		if err := c.Close(); err != nil {
			t.Logf("warning: failed to close redis cleanup client: %v", err)
		}
	})
}

// SetupTestRedis creates a Redis client for testing with automatic address detection.
// Tests will be skipped if Redis is not available.
func SetupTestRedis(t TestingTB) *redis.Client {
	t.Helper()

	addr, ok := GetTestRedisAddr(t)
	if !ok {
		if requireRedis() {
			t.Fatal("Redis not available for testing")
		}
		t.Skip("Redis not available for testing")
	}

	dbIndex := selectTestRedisDB(t, addr)
	client := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   dbIndex,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		if cerr := client.Close(); cerr != nil {
			t.Logf("warning: failed to close redis client after ping error: %v", cerr)
		}
		if requireRedis() {
			t.Fatalf("Redis not available for testing at %s: %v", addr, err)
		}
		t.Skipf("Redis not available for testing at %s: %v", addr, err)
	}

	// Clean up any existing test data
	client.FlushDB(ctx)

	return client
}

// Common pointer helper functions for tests.

// StringPtr returns a pointer to the given string value.
func StringPtr(s string) *string {
	return &s
}

// BoolPtr returns a pointer to the given bool value.
func BoolPtr(b bool) *bool {
	return &b
}

// IntPtr returns a pointer to the given int value.
func IntPtr(i int) *int {
	return &i
}

// TimePtr returns a pointer to the given time value.
func TimePtr(t time.Time) *time.Time {
	return &t
}
