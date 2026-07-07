// This file is a documentation template and should not be compiled.
// It uses placeholder types (ExampleService, ExampleRepository, etc.) that don't exist.
// Use this as a reference when creating new services.
//
//go:build ignore

package service

// TEMPLATE.go - Service Layer Pattern Template
//
// This file demonstrates the standard pattern for all services in the service layer.
// Use this as a reference when creating new services or migrating services from internal/core.
//
// KEY PRINCIPLES:
// 1. All services use Options struct pattern for dependency injection
// 2. Options structs have ≤3 fields (use nested structs if more config needed)
// 3. All services have a constructor: NewXService(opts XServiceOptions) *XService
// 4. Services depend on port interfaces (repositories), not concrete implementations
// 5. Required dependencies are validated in constructor (panic if nil)
// 6. Optional dependencies are checked for nil before use
// 7. All methods accept context.Context as first parameter
// 8. Errors are wrapped with context using fmt.Errorf("operation: %w", err)
// 9. Business logic and orchestration belong in the service layer
// 10. Services never import from internal/data, internal/adapters, or internal/http

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 1: Options Struct (≤3 fields)
// ═══════════════════════════════════════════════════════════════════════════

// ExampleServiceOptions groups dependencies for ExampleService.
//
// RULES:
// - Maximum 3 fields per options struct
// - If you need more than 3 dependencies, create a nested config struct
// - Required dependencies should be repository interfaces
// - Optional dependencies should be clearly documented
// - Use meaningful field names (not abbreviations unless obvious)
type ExampleServiceOptions struct {
	Repo   core.ExampleRepository // Required: primary repository
	Logger *slog.Logger           // Optional: structured logger
	Cache  exampleCache           // Optional: cache interface (if needed)
}

// Example with nested config struct (when you have >3 fields):
//
// type ExampleServiceConfig struct {
//     MaxBatchSize int
//     TimeoutSeconds int
//     RetryAttempts int
// }
//
// type ExampleServiceOptions struct {
//     Repo   core.ExampleRepository
//     Config ExampleServiceConfig  // Group related config
//     Logger *slog.Logger
// }

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 2: Optional Interface Dependencies
// ═══════════════════════════════════════════════════════════════════════════

// exampleCache defines the minimal behavior required from a cache service.
// Define interfaces for optional dependencies to avoid tight coupling.
type exampleCache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 3: Service Struct (private fields)
// ═══════════════════════════════════════════════════════════════════════════

// ExampleService provides business logic for example domain operations.
//
// RESPONSIBILITIES:
// - CRUD operations with business logic
// - Cross-repository orchestration
// - Transaction management (if needed)
// - Caching strategies
// - Async operations (goroutines, pub/sub)
// - Business rule enforcement
//
// DOES NOT:
// - Import from internal/data (depends on interfaces only)
// - Import from internal/http (transport layer depends on service, not vice versa)
// - Import from internal/adapters (adapters depend on service, not vice versa)
type ExampleService struct {
	repo   core.ExampleRepository // Required dependency
	logger *slog.Logger           // Optional dependency
	cache  exampleCache           // Optional dependency
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 4: Constructor with Validation
// ═══════════════════════════════════════════════════════════════════════════

// NewExampleService constructs a new ExampleService.
//
// RULES:
// - Validate required dependencies (panic if nil)
// - Optional dependencies can be nil (check before use)
// - Return pointer to service struct
// - Keep constructor simple (no complex logic)
func NewExampleService(opts ExampleServiceOptions) *ExampleService {
	// Validate required dependencies
	if opts.Repo == nil {
		panic("ExampleRepository is required")
	}

	// Optional: Log service initialization
	if opts.Logger != nil {
		opts.Logger.Info("ExampleService initialized")
	}

	return &ExampleService{
		repo:   opts.Repo,
		logger: opts.Logger,
		cache:  opts.Cache,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 5: Simple CRUD Operations
// ═══════════════════════════════════════════════════════════════════════════

// Create creates a new example entity.
//
// RULES:
// - Accept context.Context as first parameter
// - Use request types from internal/domain/model
// - Wrap errors with context: fmt.Errorf("operation: %w", err)
// - Add business logic/validation before calling repository
// - Return domain types from internal/domain/model
func (s *ExampleService) Create(
	ctx context.Context,
	req model.CreateExampleRequest,
) (*model.Example, error) {
	// Optional: Business logic validation
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	// Optional: Normalize request
	req.Normalize()

	// Call repository
	example, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create example: %w", err)
	}

	// Optional: Log success
	if s.logger != nil {
		s.logger.Info("example created", "id", example.ID, "name", example.Name)
	}

	return example, nil
}

// GetByID retrieves an example entity by ID.
func (s *ExampleService) GetByID(ctx context.Context, id string) (*model.Example, error) {
	// Optional: Check cache first
	if s.cache != nil {
		if cached, err := s.getCached(ctx, id); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Fetch from repository
	example, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get example by id: %w", err)
	}

	// Optional: Cache result
	if s.cache != nil {
		_ = s.setCached(ctx, example) // Best-effort caching
	}

	return example, nil
}

// List retrieves a paginated list of examples.
func (s *ExampleService) List(
	ctx context.Context,
	limit int,
	offset int,
) ([]*model.Example, error) {
	// Optional: Normalize pagination parameters
	if limit <= 0 {
		limit = 50 // Default
	}
	if limit > 1000 {
		limit = 1000 // Max
	}

	return s.repo.List(ctx, limit, offset)
}

// Update updates an existing example entity.
func (s *ExampleService) Update(
	ctx context.Context,
	id string,
	req model.UpdateExampleRequest,
) (*model.Example, error) {
	// Optional: Business logic validation
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	// Call repository
	example, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update example: %w", err)
	}

	// Optional: Invalidate cache
	if s.cache != nil {
		_ = s.invalidateCache(ctx, id) // Best-effort
	}

	return example, nil
}

// Delete deletes an example entity by ID.
func (s *ExampleService) Delete(ctx context.Context, id string) (bool, error) {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete example: %w", err)
	}

	// Optional: Invalidate cache
	if s.cache != nil && deleted {
		_ = s.invalidateCache(ctx, id) // Best-effort
	}

	return deleted, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 6: Orchestration Across Multiple Repositories
// ═══════════════════════════════════════════════════════════════════════════

// CreateWithRelated demonstrates orchestration across multiple repositories.
// This is where the service layer adds value beyond simple CRUD.
func (s *ExampleService) CreateWithRelated(
	ctx context.Context,
	req model.CreateExampleWithRelatedRequest,
) (*model.Example, error) {
	// Step 1: Create main entity
	example, err := s.repo.Create(ctx, model.CreateExampleRequest{
		Name:  req.Name,
		Value: req.Value,
	})
	if err != nil {
		return nil, fmt.Errorf("create example: %w", err)
	}

	// Step 2: Create related entities (orchestration logic)
	// This demonstrates why we need a service layer - coordinating multiple operations
	if req.AutoCreateRelated {
		if err := s.createRelatedEntities(ctx, example); err != nil {
			// Note: In production, you might want to rollback or use transactions
			return nil, fmt.Errorf("create related entities: %w", err)
		}
	}

	return example, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 7: Private Helper Methods
// ═══════════════════════════════════════════════════════════════════════════

// Private helper methods should be lowercase and focused on single responsibility.
// These encapsulate implementation details and keep public methods clean.

func (s *ExampleService) getCached(ctx context.Context, id string) (*model.Example, error) {
	// Implementation details hidden from public API
	return nil, nil // Placeholder
}

func (s *ExampleService) setCached(ctx context.Context, example *model.Example) error {
	// Implementation details hidden from public API
	return nil // Placeholder
}

func (s *ExampleService) invalidateCache(ctx context.Context, id string) error {
	// Implementation details hidden from public API
	return nil // Placeholder
}

func (s *ExampleService) createRelatedEntities(
	ctx context.Context,
	example *model.Example,
) error {
	// Orchestration logic for creating related entities
	// This might involve calling other repositories or services
	return nil // Placeholder
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 8: Optional Repository Extensions (Type Assertions)
// ═══════════════════════════════════════════════════════════════════════════

// Some repositories may support optional extensions. Use type assertions to check.
// This allows graceful degradation when features aren't available.

type exampleRepositoryWithSearch interface {
	SearchByName(ctx context.Context, query string, limit int) ([]*model.Example, error)
}

// SearchByName searches examples by name if the repository supports it.
// Falls back to regular list if not supported.
func (s *ExampleService) SearchByName(
	ctx context.Context,
	query string,
	limit int,
) ([]*model.Example, error) {
	// Check if repository supports search
	if repo, ok := any(s.repo).(exampleRepositoryWithSearch); ok {
		return repo.SearchByName(ctx, query, limit)
	}

	// Fallback to unfiltered list
	if s.logger != nil {
		s.logger.Debug("repository does not support search, falling back to list")
	}
	return s.repo.List(ctx, limit, 0)
}

// ═══════════════════════════════════════════════════════════════════════════
// NOTES FOR MIGRATION
// ═══════════════════════════════════════════════════════════════════════════
//
// When migrating from internal/core/*_service.go:
//
// 1. Copy the service struct and methods to internal/service/
// 2. Update to use Options pattern with constructor
// 3. Add logger as optional dependency
// 4. Ensure all repository dependencies use interfaces from internal/core
// 5. Update all references in HTTP handlers, adapters, and main.go
// 6. Migrate unit tests to service package
// 7. Run integration tests to verify no breakage
// 8. Delete the old core service file
//
// Common pitfalls:
// - Forgetting to validate required dependencies in constructor
// - Not wrapping errors with context
// - Importing from internal/data (use interfaces instead)
// - Creating functions with >3 parameters (use structs)
// - Not checking optional dependencies for nil before use
