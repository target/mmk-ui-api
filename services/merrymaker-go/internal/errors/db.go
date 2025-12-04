package errors

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Regular expressions for parsing PgError.Detail messages.
var (
	// reKeyField extracts field name from unique violation detail: "Key (field)=(value) already exists.".
	reKeyField = regexp.MustCompile(`Key \(([^)]+)\)=`)
	// reReferencedFrom detects parent deletion: "... is still referenced from table ...".
	reReferencedFrom = regexp.MustCompile(`is still referenced from table "?([^"]+)"?`)
	// reNotPresent detects missing parent: "... is not present in table ...".
	reNotPresent = regexp.MustCompile(`is not present in table "?([^"]+)"?`)
)

// MapDBError maps database errors to AppError instances.
// It handles common database error patterns including:
// - pgx.ErrNoRows → NotFound
// - Unique constraint violations → Conflict
// - Foreign key violations → ForeignKey
// - Check constraint violations → Validation
// - NOT NULL violations → Validation
// - Context timeouts/cancellations → Timeout/Canceled
//
// If the error is not a recognized database error, it returns the original error.
func MapDBError(err error) error {
	if err == nil {
		return nil
	}

	// Check for context errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return &AppError{
			Code:    ErrCodeTimeout,
			Message: "Request timed out. Please try again.",
			Cause:   err,
		}
	}
	if errors.Is(err, context.Canceled) {
		return &AppError{
			Code:    ErrCodeCanceled,
			Message: "Request was canceled.",
			Cause:   err,
		}
	}

	// Check for pgx.ErrNoRows (not found)
	if errors.Is(err, pgx.ErrNoRows) {
		return &AppError{
			Code:    ErrCodeNotFound,
			Message: "Resource not found",
			Cause:   err,
		}
	}

	// Check for PostgreSQL errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return mapPgError(pgErr)
	}

	// Return original error if not a recognized database error
	return err
}

// mapPgError maps PostgreSQL-specific errors to AppError instances.
func mapPgError(pgErr *pgconn.PgError) error {
	switch pgErr.Code {
	case pgerrcode.UniqueViolation:
		return mapUniqueViolation(pgErr)
	case pgerrcode.ForeignKeyViolation:
		return mapForeignKeyViolation(pgErr)
	case pgerrcode.CheckViolation:
		return mapCheckViolation(pgErr)
	case pgerrcode.NotNullViolation:
		return mapNotNullViolation(pgErr)
	default:
		// Return wrapped internal error for unhandled database errors
		return &AppError{
			Code:    ErrCodeInternal,
			Message: "A database error occurred. Please try again.",
			Cause:   pgErr,
		}
	}
}

// mapUniqueViolation maps unique constraint violations to Conflict errors.
func mapUniqueViolation(pgErr *pgconn.PgError) error {
	var field string

	// Prefer ColumnName metadata when available (most reliable)
	if pgErr.ColumnName != "" {
		field = pgErr.ColumnName
	}

	// Fallback: Parse Detail message for "Key (field)=(value) already exists."
	// This is more reliable than constraint name inference for multi-column and non-standard constraints
	if field == "" && pgErr.Detail != "" {
		if m := reKeyField.FindStringSubmatch(pgErr.Detail); len(m) == 2 {
			field = m[1]
		}
	}

	// Last resort: Infer from constraint name (e.g., "secrets_name_key" → "name")
	if field == "" {
		field = inferFieldFromConstraint(pgErr.ConstraintName)
	}

	message := "This value already exists. Please choose a different one."
	if field != "" {
		return &AppError{
			Code:    ErrCodeConflict,
			Message: message,
			Field:   field,
			Cause:   pgErr,
		}
	}

	return &AppError{
		Code:    ErrCodeConflict,
		Message: message,
		Cause:   pgErr,
	}
}

// mapForeignKeyViolation maps foreign key constraint violations to ForeignKey errors.
func mapForeignKeyViolation(pgErr *pgconn.PgError) error {
	var message string

	// Parse Detail message to distinguish between:
	// - "... is still referenced from table ..." (deleting/updating parent)
	// - "... is not present in table ..." (inserting/updating child with missing parent)
	if pgErr.Detail != "" {
		if referencedMatch := reReferencedFrom.FindStringSubmatch(pgErr.Detail); len(referencedMatch) == 2 {
			// Parent deletion: "is still referenced from table X"
			tableName := referencedMatch[1]
			domainName := mapTableToDomain(tableName)
			message = "Cannot delete because this item is in use by " + domainName + "."
		} else if missingMatch := reNotPresent.FindStringSubmatch(pgErr.Detail); len(missingMatch) == 2 {
			// Child insert/update with missing parent: "is not present in table X"
			tableName := missingMatch[1]
			domainName := mapTableToDomain(tableName)
			message = "Cannot complete operation because the referenced " + domainName + " does not exist."
		}
	}

	// Fallback: Use TableName metadata if Detail parsing failed
	if message == "" && pgErr.TableName != "" {
		domainName := mapTableToDomain(pgErr.TableName)
		message = "Cannot complete operation because this item is in use by " + domainName + "."
	}

	// Last resort: Infer from constraint name
	if message == "" {
		message = inferForeignKeyMessage(pgErr.ConstraintName)
	}

	return &AppError{
		Code:    ErrCodeForeignKey,
		Message: message,
		Cause:   pgErr,
	}
}

// mapNotNullViolation maps NOT NULL constraint violations to Validation errors.
func mapNotNullViolation(pgErr *pgconn.PgError) error {
	message := "Required field is missing. Please check your input."
	field := pgErr.ColumnName

	if field != "" {
		return &AppError{
			Code:    ErrCodeValidation,
			Message: "This field is required.",
			Field:   field,
			Cause:   pgErr,
		}
	}

	return &AppError{
		Code:    ErrCodeValidation,
		Message: message,
		Cause:   pgErr,
	}
}

// mapCheckViolation maps CHECK constraint violations to Validation errors.
func mapCheckViolation(pgErr *pgconn.PgError) error {
	message := "Invalid data. Please check your input."
	field := pgErr.ColumnName

	if field != "" {
		return &AppError{
			Code:    ErrCodeValidation,
			Message: "This field has an invalid value.",
			Field:   field,
			Cause:   pgErr,
		}
	}

	return &AppError{
		Code:    ErrCodeValidation,
		Message: message,
		Cause:   pgErr,
	}
}

// inferFieldFromConstraint attempts to infer the field name from a constraint name.
// e.g., "secrets_name_key" → "name"
// e.g., "sites_name_unique" → "name"
// Returns empty string if inference fails or is ambiguous.
func inferFieldFromConstraint(constraintName string) string {
	if constraintName == "" {
		return ""
	}

	parts := strings.Split(constraintName, "_")
	// Constraint names typically follow patterns like:
	// - "table_field_key" (unique) → 3 parts
	// - "table_field_unique" → 3 parts
	// - "table_field_idx" → 3 parts

	// Multi-column or complex constraints have more parts
	// e.g., "table_field1_field2_key" → 4+ parts
	// Avoid returning misleading field names for these cases
	if len(parts) > 3 {
		return "" // Ambiguous: could be multi-column or expression index
	}

	if len(parts) == 3 {
		fieldCandidate := parts[1] // The middle segment

		// Check if this looks like a function name (common in expression indexes)
		// e.g., "table_lower_key" where "lower" is a function, not a field
		if isFunctionName(fieldCandidate) {
			return "" // Expression index, not a direct field
		}

		return fieldCandidate
	}

	return "" // Not enough parts to infer
}

// mapTableToDomain maps internal table names to user-friendly domain names.
func mapTableToDomain(tableName string) string {
	// Normalize table name
	tableName = strings.ToLower(strings.TrimSpace(tableName))

	// Map common table names to domain names
	domainMap := map[string]string{
		"sources":                 "Source",
		"sites":                   "Site",
		"secrets":                 "Secret",
		"http_alert_sinks":        "HTTP Alert Sink",
		"alerts":                  "Alert",
		"jobs":                    "Job",
		"events":                  "Event",
		"source_secrets":          "Source",
		"http_alert_sink_secrets": "HTTP Alert Sink",
	}

	// Look up in map
	if domainName, ok := domainMap[tableName]; ok {
		return domainName
	}

	// Fallback: capitalize first letter and replace underscores with spaces
	return capitalizeFirst(strings.ReplaceAll(tableName, "_", " "))
}

// capitalizeFirst capitalizes the first letter of each word in a string.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}

	// Split by spaces and capitalize each word
	words := strings.Split(s, " ")
	for i, word := range words {
		if len(word) > 0 && word[0] >= 'a' && word[0] <= 'z' {
			words[i] = string(word[0]-32) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// inferForeignKeyMessage infers a user-friendly message from a foreign key constraint name.
func inferForeignKeyMessage(constraintName string) string {
	constraintName = strings.ToLower(constraintName)

	// Check for secret first (before source, since "sources_secret_id_fkey" contains both)
	if strings.Contains(constraintName, "secret") {
		return "Cannot delete secret because it is in use by a Source or HTTP Alert Sink."
	}

	// Check for alert/sink before site (since "sites_alert_sink_id_fkey" contains both)
	if strings.Contains(constraintName, "alert") || strings.Contains(constraintName, "sink") {
		return "Cannot delete because it is in use by an HTTP Alert Sink."
	}

	// Common patterns for delete operations
	if strings.Contains(constraintName, "source") {
		return "Cannot delete because it is in use by a Source."
	}
	if strings.Contains(constraintName, "site") {
		return "Cannot delete because it is in use by a Site."
	}

	// Generic foreign key violation message
	return "Cannot complete operation because this item is in use."
}

// isFunctionName checks if a string looks like a common SQL function name
// used in expression indexes (e.g., lower, upper, trim, etc.)
func isFunctionName(s string) bool {
	commonFunctions := []string{
		"lower", "upper", "trim", "ltrim", "rtrim",
		"md5", "sha1", "sha256", "encode", "decode",
	}
	s = strings.ToLower(s)
	for _, fn := range commonFunctions {
		if s == fn {
			return true
		}
	}
	return false
}
