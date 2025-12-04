package errors

import (
	"errors"
	"fmt"
)

// ErrorCode represents a category of application error.
type ErrorCode string

const (
	// ErrCodeNotFound indicates a resource was not found.
	ErrCodeNotFound ErrorCode = "not_found"
	// ErrCodeConflict indicates a conflict with existing data (e.g., unique constraint violation).
	ErrCodeConflict ErrorCode = "conflict"
	// ErrCodeValidation indicates invalid input data.
	ErrCodeValidation ErrorCode = "validation"
	// ErrCodeForeignKey indicates a foreign key constraint violation.
	ErrCodeForeignKey ErrorCode = "foreign_key"
	// ErrCodeInternal indicates an internal server error.
	ErrCodeInternal ErrorCode = "internal"
	// ErrCodeTimeout indicates a timeout occurred.
	ErrCodeTimeout ErrorCode = "timeout"
	// ErrCodeCanceled indicates the operation was canceled.
	ErrCodeCanceled ErrorCode = "canceled"
)

// AppError represents a structured application error with a code, message, and optional cause.
// It supports error wrapping and unwrapping for use with errors.Is and errors.As.
type AppError struct {
	// Code categorizes the error type
	Code ErrorCode
	// Message is a human-readable error message
	Message string
	// Cause is the underlying error that caused this error (optional)
	Cause error
	// Field is the specific field that caused the error (optional, for validation errors)
	Field string
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// NotFound creates a new NotFound error.
func NotFound(message string) *AppError {
	return &AppError{
		Code:    ErrCodeNotFound,
		Message: message,
	}
}

// NotFoundf creates a new NotFound error with formatted message.
func NotFoundf(format string, args ...any) *AppError {
	return &AppError{
		Code:    ErrCodeNotFound,
		Message: fmt.Sprintf(format, args...),
	}
}

// Conflict creates a new Conflict error.
func Conflict(message string) *AppError {
	return &AppError{
		Code:    ErrCodeConflict,
		Message: message,
	}
}

// Conflictf creates a new Conflict error with formatted message.
func Conflictf(format string, args ...any) *AppError {
	return &AppError{
		Code:    ErrCodeConflict,
		Message: fmt.Sprintf(format, args...),
	}
}

// Validation creates a new Validation error.
func Validation(message string) *AppError {
	return &AppError{
		Code:    ErrCodeValidation,
		Message: message,
	}
}

// Validationf creates a new Validation error with formatted message.
func Validationf(format string, args ...any) *AppError {
	return &AppError{
		Code:    ErrCodeValidation,
		Message: fmt.Sprintf(format, args...),
	}
}

// ValidationField creates a new Validation error for a specific field.
func ValidationField(field, message string) *AppError {
	return &AppError{
		Code:    ErrCodeValidation,
		Message: message,
		Field:   field,
	}
}

// ForeignKey creates a new ForeignKey error.
func ForeignKey(message string) *AppError {
	return &AppError{
		Code:    ErrCodeForeignKey,
		Message: message,
	}
}

// ForeignKeyf creates a new ForeignKey error with formatted message.
func ForeignKeyf(format string, args ...any) *AppError {
	return &AppError{
		Code:    ErrCodeForeignKey,
		Message: fmt.Sprintf(format, args...),
	}
}

// Internal creates a new Internal error.
func Internal(message string) *AppError {
	return &AppError{
		Code:    ErrCodeInternal,
		Message: message,
	}
}

// Internalf creates a new Internal error with formatted message.
func Internalf(format string, args ...any) *AppError {
	return &AppError{
		Code:    ErrCodeInternal,
		Message: fmt.Sprintf(format, args...),
	}
}

// Wrap wraps an existing error with an AppError, preserving the cause.
func Wrap(err error, code ErrorCode, message string) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// MessageTemplate describes a lazily formatted error message used with Wrapf.
type MessageTemplate struct {
	format string
	args   []any
}

// Messagef creates a lazily formatted message template for Wrapf.
func Messagef(format string, args ...any) MessageTemplate {
	return MessageTemplate{
		format: format,
		args:   args,
	}
}

func (mt MessageTemplate) String() string {
	if len(mt.args) == 0 {
		return mt.format
	}
	return fmt.Sprintf(mt.format, mt.args...)
}

// WrapTemplate wraps an existing error with an AppError using a preconstructed message template.
func WrapTemplate(err error, code ErrorCode, template MessageTemplate) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: template.String(),
		Cause:   err,
	}
}

// Wrapf wraps an existing error with an AppError and formatted message.
func Wrapf(err error, code ErrorCode, format string, args ...any) *AppError {
	return WrapTemplate(err, code, Messagef(format, args...))
}

// isCode checks if an error has a specific error code.
func isCode(err error, code ErrorCode) bool {
	var appErr *AppError
	return errors.As(err, &appErr) && appErr.Code == code
}

// IsNotFound checks if an error is a NotFound error.
func IsNotFound(err error) bool {
	return isCode(err, ErrCodeNotFound)
}

// IsConflict checks if an error is a Conflict error.
func IsConflict(err error) bool {
	return isCode(err, ErrCodeConflict)
}

// IsValidation checks if an error is a Validation error.
func IsValidation(err error) bool {
	return isCode(err, ErrCodeValidation)
}

// IsForeignKey checks if an error is a ForeignKey error.
func IsForeignKey(err error) bool {
	return isCode(err, ErrCodeForeignKey)
}

// IsInternal checks if an error is an Internal error.
func IsInternal(err error) bool {
	return isCode(err, ErrCodeInternal)
}

// IsTimeout checks if an error is a Timeout error.
func IsTimeout(err error) bool {
	return isCode(err, ErrCodeTimeout)
}

// IsCanceled checks if an error is a Canceled error.
func IsCanceled(err error) bool {
	return isCode(err, ErrCodeCanceled)
}

// GetCode returns the ErrorCode from an error, or empty string if not an AppError.
func GetCode(err error) ErrorCode {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

// GetField returns the Field from an error, or empty string if not an AppError or no field set.
func GetField(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Field
	}
	return ""
}
