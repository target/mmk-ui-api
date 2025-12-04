package job

import (
	"errors"
	"math"
	"time"
)

// ErrInvalidDefaultLease indicates the configured default lease duration is not positive.
var ErrInvalidDefaultLease = errors.New("default lease must be positive")

// LeaseSource identifies how a lease duration was resolved.
type LeaseSource string

const (
	// LeaseSourceExplicit indicates the caller supplied a positive duration.
	LeaseSourceExplicit LeaseSource = "explicit"
	// LeaseSourceDefault indicates the default duration was used.
	LeaseSourceDefault LeaseSource = "default"
	// LeaseSourceClamped indicates the requested duration was clamped to the minimum supported value.
	LeaseSourceClamped LeaseSource = "clamped"
)

// LeasePolicy normalises lease durations for job reservations and heartbeats.
type LeasePolicy struct {
	defaultLease time.Duration
}

// NewLeasePolicy constructs a LeasePolicy with the provided default lease duration.
func NewLeasePolicy(defaultLease time.Duration) (*LeasePolicy, error) {
	if defaultLease <= 0 {
		return nil, ErrInvalidDefaultLease
	}
	return &LeasePolicy{
		defaultLease: defaultLease,
	}, nil
}

// Default returns the configured default lease duration.
func (p *LeasePolicy) Default() time.Duration {
	if p == nil {
		return 0
	}
	return p.defaultLease
}

// LeaseDecision captures the outcome of resolving a lease request.
type LeaseDecision struct {
	Seconds   int
	Source    LeaseSource
	Requested time.Duration
}

// UsedDefault reports whether the policy fell back to the default lease.
func (d LeaseDecision) UsedDefault() bool {
	return d.Source == LeaseSourceDefault
}

// Clamped reports whether the requested value was clamped to the minimum supported duration.
func (d LeaseDecision) Clamped() bool {
	return d.Source == LeaseSourceClamped
}

// Resolve normalises the requested duration to a whole number of seconds.
func (p *LeasePolicy) Resolve(request time.Duration) LeaseDecision {
	if p == nil {
		return LeaseDecision{Seconds: 0, Source: LeaseSourceDefault, Requested: request}
	}

	decision := LeaseDecision{Requested: request}

	switch {
	case request > 0:
		seconds, clamped := durationToSeconds(request)
		decision.Seconds = seconds
		if clamped {
			decision.Source = LeaseSourceClamped
		} else {
			decision.Source = LeaseSourceExplicit
		}
		return decision
	case request == 0:
		seconds, _ := durationToSeconds(p.defaultLease)
		decision.Seconds = seconds
		decision.Source = LeaseSourceDefault
		return decision
	default:
		decision.Seconds = 1
		decision.Source = LeaseSourceClamped
		return decision
	}
}

func durationToSeconds(d time.Duration) (int, bool) {
	seconds := int64(d / time.Second)
	clamped := false

	if seconds <= 0 {
		seconds = 1
		clamped = true
	}

	maxSeconds := int64(math.MaxInt)
	if seconds > maxSeconds {
		seconds = maxSeconds
		clamped = true
	}

	return int(seconds), clamped
}
