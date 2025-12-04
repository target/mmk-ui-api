package data

import "time"

// TimeProvider provides time-related functionality that can be mocked for testing.
type TimeProvider interface {
	// Now returns the current time
	Now() time.Time
	// FormatForDB formats a time for database insertion
	FormatForDB(t time.Time) string
}

// RealTimeProvider implements TimeProvider using real system time.
type RealTimeProvider struct{}

// Now returns the current system time.
func (r *RealTimeProvider) Now() time.Time {
	return time.Now()
}

// FormatForDB formats a time for PostgreSQL insertion.
func (r *RealTimeProvider) FormatForDB(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FixedTimeProvider implements TimeProvider with a fixed time for testing.
type FixedTimeProvider struct {
	fixedTime time.Time
}

// NewFixedTimeProvider creates a new FixedTimeProvider with the given time.
func NewFixedTimeProvider(t time.Time) *FixedTimeProvider {
	return &FixedTimeProvider{fixedTime: t}
}

// Now returns the fixed time.
func (f *FixedTimeProvider) Now() time.Time {
	return f.fixedTime
}

// FormatForDB formats the fixed time for PostgreSQL insertion.
func (f *FixedTimeProvider) FormatForDB(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// SetTime updates the fixed time (useful for testing time progression).
func (f *FixedTimeProvider) SetTime(t time.Time) {
	f.fixedTime = t
}

// AddTime adds a duration to the current fixed time.
func (f *FixedTimeProvider) AddTime(d time.Duration) {
	f.fixedTime = f.fixedTime.Add(d)
}
