package errors

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapDBError_NilError(t *testing.T) {
	err := MapDBError(nil)
	if err != nil {
		t.Errorf("MapDBError(nil) = %v, want nil", err)
	}
}

func TestMapDBError_ContextErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode ErrorCode
	}{
		{
			name:     "deadline exceeded",
			err:      context.DeadlineExceeded,
			wantCode: ErrCodeTimeout,
		},
		{
			name:     "canceled",
			err:      context.Canceled,
			wantCode: ErrCodeCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapDBError(tt.err)
			if !IsAppError(err, tt.wantCode) {
				t.Errorf("MapDBError() code = %v, want %v", GetCode(err), tt.wantCode)
			}
		})
	}
}

func TestMapDBError_NoRows(t *testing.T) {
	err := MapDBError(pgx.ErrNoRows)
	if !IsNotFound(err) {
		t.Errorf("MapDBError(pgx.ErrNoRows) should be NotFound, got %v", GetCode(err))
	}
}

func TestMapDBError_UniqueViolation(t *testing.T) {
	tests := []struct {
		name      string
		pgErr     *pgconn.PgError
		wantCode  ErrorCode
		wantField string
	}{
		{
			name: "unique violation with column name",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.UniqueViolation,
				ConstraintName: "secrets_name_key",
				ColumnName:     "name",
			},
			wantCode:  ErrCodeConflict,
			wantField: "name",
		},
		{
			name: "unique violation with Detail message",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.UniqueViolation,
				ConstraintName: "secrets_name_key",
				Detail:         `Key (name)=(test-secret) already exists.`,
			},
			wantCode:  ErrCodeConflict,
			wantField: "name", // extracted from Detail
		},
		{
			name: "unique violation with multi-column Detail",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.UniqueViolation,
				ConstraintName: "table_field1_field2_key",
				Detail:         `Key (field1, field2)=(val1, val2) already exists.`,
			},
			wantCode:  ErrCodeConflict,
			wantField: "field1, field2", // extracted from Detail
		},
		{
			name: "unique violation without column name",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.UniqueViolation,
				ConstraintName: "secrets_name_key",
			},
			wantCode:  ErrCodeConflict,
			wantField: "name", // inferred from constraint name
		},
		{
			name: "unique violation with ambiguous constraint",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.UniqueViolation,
				ConstraintName: "table_field1_field2_key",
			},
			wantCode:  ErrCodeConflict,
			wantField: "", // cannot infer from multi-column constraint
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapDBError(tt.pgErr)
			if !IsConflict(err) {
				t.Errorf("MapDBError() should be Conflict, got %v", GetCode(err))
			}
			if field := GetField(err); field != tt.wantField {
				t.Errorf("MapDBError() field = %v, want %v", field, tt.wantField)
			}
		})
	}
}

func TestMapDBError_ForeignKeyViolation(t *testing.T) {
	tests := []struct {
		name         string
		pgErr        *pgconn.PgError
		wantCode     ErrorCode
		wantContains string
	}{
		{
			name: "foreign key violation - parent deletion (Detail)",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.ForeignKeyViolation,
				ConstraintName: "sources_secret_id_fkey",
				Detail:         `Key (id)=(secret-123) is still referenced from table "sources".`,
			},
			wantCode:     ErrCodeForeignKey,
			wantContains: "in use by Source",
		},
		{
			name: "foreign key violation - missing parent (Detail)",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.ForeignKeyViolation,
				ConstraintName: "sources_secret_id_fkey",
				Detail:         `Key (secret_id)=(secret-123) is not present in table "secrets".`,
			},
			wantCode:     ErrCodeForeignKey,
			wantContains: "does not exist",
		},
		{
			name: "foreign key violation with table name",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.ForeignKeyViolation,
				ConstraintName: "sources_secret_id_fkey",
				TableName:      "sources",
			},
			wantCode:     ErrCodeForeignKey,
			wantContains: "Source",
		},
		{
			name: "foreign key violation without table name",
			pgErr: &pgconn.PgError{
				Code:           pgerrcode.ForeignKeyViolation,
				ConstraintName: "sources_secret_id_fkey",
			},
			wantCode:     ErrCodeForeignKey,
			wantContains: "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapDBError(tt.pgErr)
			if !IsForeignKey(err) {
				t.Errorf("MapDBError() should be ForeignKey, got %v", GetCode(err))
			}
			var appErr *AppError
			if errors.As(err, &appErr) {
				msgLower := strings.ToLower(appErr.Message)
				wantLower := strings.ToLower(tt.wantContains)
				if !strings.Contains(msgLower, wantLower) {
					t.Errorf("MapDBError() message = %q, want to contain %q", appErr.Message, tt.wantContains)
				}
			}
		})
	}
}

func TestMapDBError_NotNullViolation(t *testing.T) {
	tests := []struct {
		name      string
		pgErr     *pgconn.PgError
		wantCode  ErrorCode
		wantField string
	}{
		{
			name: "not null violation with column name",
			pgErr: &pgconn.PgError{
				Code:       pgerrcode.NotNullViolation,
				ColumnName: "name",
			},
			wantCode:  ErrCodeValidation,
			wantField: "name",
		},
		{
			name: "not null violation without column name",
			pgErr: &pgconn.PgError{
				Code: pgerrcode.NotNullViolation,
			},
			wantCode:  ErrCodeValidation,
			wantField: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapDBError(tt.pgErr)
			if !IsValidation(err) {
				t.Errorf("MapDBError() should be Validation, got %v", GetCode(err))
			}
			if field := GetField(err); field != tt.wantField {
				t.Errorf("MapDBError() field = %v, want %v", field, tt.wantField)
			}
		})
	}
}

func TestMapDBError_CheckViolation(t *testing.T) {
	tests := []struct {
		name      string
		pgErr     *pgconn.PgError
		wantCode  ErrorCode
		wantField string
	}{
		{
			name: "check violation with column name",
			pgErr: &pgconn.PgError{
				Code:       pgerrcode.CheckViolation,
				ColumnName: "age",
			},
			wantCode:  ErrCodeValidation,
			wantField: "age",
		},
		{
			name: "check violation without column name",
			pgErr: &pgconn.PgError{
				Code: pgerrcode.CheckViolation,
			},
			wantCode:  ErrCodeValidation,
			wantField: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapDBError(tt.pgErr)
			if !IsValidation(err) {
				t.Errorf("MapDBError() should be Validation, got %v", GetCode(err))
			}
			if field := GetField(err); field != tt.wantField {
				t.Errorf("MapDBError() field = %v, want %v", field, tt.wantField)
			}
		})
	}
}

func TestMapDBError_UnknownPgError(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "99999", // unknown error code
		Message: "unknown error",
	}
	err := MapDBError(pgErr)
	if !IsInternal(err) {
		t.Errorf("MapDBError() should be Internal for unknown pg error, got %v", GetCode(err))
	}
}

func TestMapDBError_StandardError(t *testing.T) {
	stdErr := errors.New("standard error")
	err := MapDBError(stdErr)
	if !errors.Is(err, stdErr) {
		t.Errorf("MapDBError() should return original error for non-db errors, got %v", err)
	}
}

func TestInferFieldFromConstraint(t *testing.T) {
	tests := []struct {
		name           string
		constraintName string
		want           string
	}{
		{
			name:           "simple unique constraint",
			constraintName: "secrets_name_key",
			want:           "name",
		},
		{
			name:           "unique constraint with unique suffix",
			constraintName: "sites_name_unique",
			want:           "name",
		},
		{
			name:           "multi-column constraint",
			constraintName: "table_field1_field2_key",
			want:           "", // ambiguous
		},
		{
			name:           "expression index",
			constraintName: "table_lower_key",
			want:           "", // function name, not field
		},
		{
			name:           "empty constraint name",
			constraintName: "",
			want:           "",
		},
		{
			name:           "too few parts",
			constraintName: "table_key",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferFieldFromConstraint(tt.constraintName); got != tt.want {
				t.Errorf("inferFieldFromConstraint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInferForeignKeyMessage(t *testing.T) {
	tests := []struct {
		name           string
		constraintName string
		wantContains   string
	}{
		{
			name:           "secret constraint",
			constraintName: "sources_secret_id_fkey",
			wantContains:   "secret",
		},
		{
			name:           "alert constraint",
			constraintName: "sites_alert_sink_id_fkey",
			wantContains:   "Alert Sink",
		},
		{
			name:           "source constraint",
			constraintName: "jobs_source_id_fkey",
			wantContains:   "Source",
		},
		{
			name:           "site constraint",
			constraintName: "jobs_site_id_fkey",
			wantContains:   "Site",
		},
		{
			name:           "unknown constraint",
			constraintName: "unknown_fkey",
			wantContains:   "in use",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferForeignKeyMessage(tt.constraintName)
			if got == "" {
				t.Errorf("inferForeignKeyMessage() returned empty string")
			}
			// Assert that the message contains the expected substring (case-insensitive)
			gotLower := strings.ToLower(got)
			wantLower := strings.ToLower(tt.wantContains)
			if !strings.Contains(gotLower, wantLower) {
				t.Errorf("inferForeignKeyMessage() = %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestIsFunctionName(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "lower", s: "lower", want: true},
		{name: "upper", s: "upper", want: true},
		{name: "LOWER (uppercase)", s: "LOWER", want: true},
		{name: "not a function", s: "name", want: false},
		{name: "empty", s: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFunctionName(tt.s); got != tt.want {
				t.Errorf("isFunctionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapTableToDomain(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		want      string
	}{
		{name: "sources", tableName: "sources", want: "Source"},
		{name: "sites", tableName: "sites", want: "Site"},
		{name: "secrets", tableName: "secrets", want: "Secret"},
		{name: "http_alert_sinks", tableName: "http_alert_sinks", want: "HTTP Alert Sink"},
		{name: "alerts", tableName: "alerts", want: "Alert"},
		{name: "jobs", tableName: "jobs", want: "Job"},
		{name: "events", tableName: "events", want: "Event"},
		{name: "source_secrets", tableName: "source_secrets", want: "Source"},
		{name: "http_alert_sink_secrets", tableName: "http_alert_sink_secrets", want: "HTTP Alert Sink"},
		{name: "uppercase", tableName: "SOURCES", want: "Source"},
		{name: "with spaces", tableName: "  sources  ", want: "Source"},
		{name: "unknown table", tableName: "unknown_table", want: "Unknown Table"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapTableToDomain(tt.tableName); got != tt.want {
				t.Errorf("mapTableToDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function for tests.
func IsAppError(err error, code ErrorCode) bool {
	return GetCode(err) == code
}
