package errors

import (
	"errors"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		want string
	}{
		{
			name: "error without cause",
			err: &AppError{
				Code:    ErrCodeNotFound,
				Message: "resource not found",
			},
			want: "resource not found",
		},
		{
			name: "error with cause",
			err: &AppError{
				Code:    ErrCodeInternal,
				Message: "failed to process",
				Cause:   errors.New("underlying error"),
			},
			want: "failed to process: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("AppError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &AppError{
		Code:    ErrCodeInternal,
		Message: "wrapped error",
		Cause:   cause,
	}

	if unwrapped := err.Unwrap(); !errors.Is(unwrapped, cause) {
		t.Errorf("AppError.Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestNotFound(t *testing.T) {
	err := NotFound("resource not found")
	if err.Code != ErrCodeNotFound {
		t.Errorf("NotFound().Code = %v, want %v", err.Code, ErrCodeNotFound)
	}
	if err.Message != "resource not found" {
		t.Errorf("NotFound().Message = %v, want %v", err.Message, "resource not found")
	}
}

func TestNotFoundf(t *testing.T) {
	err := NotFoundf("resource %s not found", "user")
	if err.Code != ErrCodeNotFound {
		t.Errorf("NotFoundf().Code = %v, want %v", err.Code, ErrCodeNotFound)
	}
	if err.Message != "resource user not found" {
		t.Errorf("NotFoundf().Message = %v, want %v", err.Message, "resource user not found")
	}
}

func TestConflict(t *testing.T) {
	err := Conflict("resource already exists")
	if err.Code != ErrCodeConflict {
		t.Errorf("Conflict().Code = %v, want %v", err.Code, ErrCodeConflict)
	}
	if err.Message != "resource already exists" {
		t.Errorf("Conflict().Message = %v, want %v", err.Message, "resource already exists")
	}
}

func TestValidation(t *testing.T) {
	err := Validation("invalid input")
	if err.Code != ErrCodeValidation {
		t.Errorf("Validation().Code = %v, want %v", err.Code, ErrCodeValidation)
	}
	if err.Message != "invalid input" {
		t.Errorf("Validation().Message = %v, want %v", err.Message, "invalid input")
	}
}

func TestValidationField(t *testing.T) {
	err := ValidationField("email", "invalid email format")
	if err.Code != ErrCodeValidation {
		t.Errorf("ValidationField().Code = %v, want %v", err.Code, ErrCodeValidation)
	}
	if err.Field != "email" {
		t.Errorf("ValidationField().Field = %v, want %v", err.Field, "email")
	}
	if err.Message != "invalid email format" {
		t.Errorf("ValidationField().Message = %v, want %v", err.Message, "invalid email format")
	}
}

func TestForeignKey(t *testing.T) {
	err := ForeignKey("resource is in use")
	if err.Code != ErrCodeForeignKey {
		t.Errorf("ForeignKey().Code = %v, want %v", err.Code, ErrCodeForeignKey)
	}
	if err.Message != "resource is in use" {
		t.Errorf("ForeignKey().Message = %v, want %v", err.Message, "resource is in use")
	}
}

func TestInternal(t *testing.T) {
	err := Internal("internal server error")
	if err.Code != ErrCodeInternal {
		t.Errorf("Internal().Code = %v, want %v", err.Code, ErrCodeInternal)
	}
	if err.Message != "internal server error" {
		t.Errorf("Internal().Message = %v, want %v", err.Message, "internal server error")
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := Wrap(cause, ErrCodeInternal, "wrapped error")

	if err.Code != ErrCodeInternal {
		t.Errorf("Wrap().Code = %v, want %v", err.Code, ErrCodeInternal)
	}
	if err.Message != "wrapped error" {
		t.Errorf("Wrap().Message = %v, want %v", err.Message, "wrapped error")
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Wrap().Cause = %v, want %v", err.Cause, cause)
	}
}

func TestWrap_NilError(t *testing.T) {
	err := Wrap(nil, ErrCodeInternal, "wrapped error")
	if err != nil {
		t.Errorf("Wrap(nil) = %v, want nil", err)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "not found error",
			err:  NotFound("resource not found"),
			want: true,
		},
		{
			name: "other error",
			err:  Conflict("conflict"),
			want: false,
		},
		{
			name: "standard error",
			err:  errors.New("standard error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "conflict error",
			err:  Conflict("conflict"),
			want: true,
		},
		{
			name: "other error",
			err:  NotFound("not found"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConflict(tt.err); got != tt.want {
				t.Errorf("IsConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "validation error",
			err:  Validation("invalid"),
			want: true,
		},
		{
			name: "validation field error",
			err:  ValidationField("email", "invalid"),
			want: true,
		},
		{
			name: "other error",
			err:  NotFound("not found"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidation(tt.err); got != tt.want {
				t.Errorf("IsValidation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "timeout error",
			err:  &AppError{Code: ErrCodeTimeout, Message: "timeout"},
			want: true,
		},
		{
			name: "other error",
			err:  NotFound("not found"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTimeout(tt.err); got != tt.want {
				t.Errorf("IsTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCanceled(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "canceled error",
			err:  &AppError{Code: ErrCodeCanceled, Message: "canceled"},
			want: true,
		},
		{
			name: "other error",
			err:  NotFound("not found"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCanceled(tt.err); got != tt.want {
				t.Errorf("IsCanceled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{
			name: "app error",
			err:  NotFound("not found"),
			want: ErrCodeNotFound,
		},
		{
			name: "standard error",
			err:  errors.New("standard error"),
			want: "",
		},
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCode(tt.err); got != tt.want {
				t.Errorf("GetCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetField(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "validation field error",
			err:  ValidationField("email", "invalid"),
			want: "email",
		},
		{
			name: "error without field",
			err:  NotFound("not found"),
			want: "",
		},
		{
			name: "standard error",
			err:  errors.New("standard error"),
			want: "",
		},
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetField(tt.err); got != tt.want {
				t.Errorf("GetField() = %v, want %v", got, tt.want)
			}
		})
	}
}
