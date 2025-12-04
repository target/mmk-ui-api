package metrics

import (
	"time"

	obserrors "github.com/target/mmk-ui-api/internal/observability/errors"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
)

// Result constants for metric tagging.
const (
	ResultSuccess = "success"
	ResultError   = "error"
	ResultNoop    = "noop"
)

// JobMetric captures details about a job lifecycle event for metric emission.
type JobMetric struct {
	JobType    string
	Transition string
	Result     string
	Duration   time.Duration
	Err        error
}

// EmitJobLifecycle emits standardised job lifecycle metrics.
func EmitJobLifecycle(sink statsd.Sink, in JobMetric) {
	if sink == nil {
		return
	}

	tags := map[string]string{
		"job_type":   in.JobType,
		"transition": in.Transition,
		"result":     in.Result,
	}

	if in.Err != nil && in.Result == ResultError {
		if class := obserrors.Classify(in.Err); class != "" {
			tags["error_class"] = class
		}
	}

	sink.Count("job.transition", 1, tags)

	if in.Duration > 0 {
		sink.Timing("job.duration", in.Duration, CloneTags(tags))
	}
}

// CloneTags creates a shallow copy of a tag map, filtering out empty keys.
func CloneTags(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
