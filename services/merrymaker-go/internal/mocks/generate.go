// Package mocks provides mock implementations for testing the merrymaker job system.
//
// This package uses go.uber.org/mock (gomock) to generate type-safe mocks for our repository interfaces.
// The mocks are generated using go:generate directives and provide a fluent API for setting up test expectations.
//
// To regenerate mocks after interface changes, run:
//
//	go generate ./internal/mocks
//
// Usage in tests:
//
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//	mockRepo := mocks.NewMockJobRepository(ctrl)
//	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(job, nil)
package mocks

// Generate mock for JobRepository interface from internal/core package.
// This creates MockJobRepository with methods for all JobRepository interface methods:
// Create, ReserveNext, WaitForNotification, Subscribe, Heartbeat, Complete, Fail, Stats
//go:generate go run go.uber.org/mock/mockgen -package=mocks -destination=job_repository_mock.go github.com/target/mmk-ui-api/internal/core JobRepository

// Generate mock for EventRepository interface from internal/core package.
// This creates MockEventRepository with methods for all EventRepository interface methods:
// BulkInsert
//go:generate go run go.uber.org/mock/mockgen -package=mocks -destination=event_repository_mock.go github.com/target/mmk-ui-api/internal/core EventRepository

// Generate mock for ScheduledJobsRepository interface from internal/core package.
// This creates MockScheduledJobsRepository with methods for all ScheduledJobsRepository interface methods:
// FindDue, MarkQueued, TryWithTaskLock
//go:generate go run go.uber.org/mock/mockgen -package=mocks -destination=scheduled_jobs_repository_mock.go github.com/target/mmk-ui-api/internal/core ScheduledJobsRepository

// Generate mock for JobIntrospector interface from internal/core package.
// This creates MockJobIntrospector with methods for all JobIntrospector interface methods:
// RunningJobExistsByTaskName
//go:generate go run go.uber.org/mock/mockgen -package=mocks -destination=job_introspector_mock.go github.com/target/mmk-ui-api/internal/core JobIntrospector

// Generate mock for SourceRepository interface from internal/core package.
// This creates MockSourceRepository with methods for all SourceRepository interface methods:
// Create, GetByID, GetByName, List, Update, Delete
//go:generate go run go.uber.org/mock/mockgen -package=mocks -destination=source_repository_mock.go github.com/target/mmk-ui-api/internal/core SourceRepository

// Generate mock for SecretRepository interface from internal/core package.
// This creates MockSecretRepository with methods for all SecretRepository interface methods:
// Create, GetByID, GetByName, List, Update, Delete
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -package=mocks -destination=secret_repository_mock.go github.com/target/mmk-ui-api/internal/core SecretRepository

// Generate mock for HTTPAlertSinkRepository interface from internal/core package.
// This creates MockHTTPAlertSinkRepository with methods for all HTTPAlertSinkRepository interface methods:
// Create, GetByID, GetByName, List, Update, Delete
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -package=mocks -destination=http_alert_sink_repository_mock.go github.com/target/mmk-ui-api/internal/core HTTPAlertSinkRepository

// Generate mock for AlertRepository interface from internal/core package.
// This creates MockAlertRepository with methods for all AlertRepository interface methods:
// Create, GetByID, List, Delete, Stats, Resolve
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -package=mocks -destination=alert_repository_mock.go github.com/target/mmk-ui-api/internal/core AlertRepository
