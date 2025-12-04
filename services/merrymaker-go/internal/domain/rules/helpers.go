package rules

import "strings"

func appendSample(samples *[]string, domain string) {
	if samples == nil {
		return
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return
	}
	for _, existing := range *samples {
		if strings.EqualFold(existing, domain) {
			return
		}
	}
	if len(*samples) < MetricsSampleLimit {
		*samples = append(*samples, domain)
	}
}

// MergeUnknownDomainMetrics merges src into dst, preserving sample limits.
func MergeUnknownDomainMetrics(dst *UnknownDomainMetrics, src UnknownDomainMetrics) {
	if dst == nil {
		return
	}
	dst.Alerted.Merge(src.Alerted)
	dst.AlertedDryRun.Merge(src.AlertedDryRun)
	dst.AlertedMuted.Merge(src.AlertedMuted)
	dst.SuppressedAllowlist.Merge(src.SuppressedAllowlist)
	dst.SuppressedSeen.Merge(src.SuppressedSeen)
	dst.SuppressedDedupe.Merge(src.SuppressedDedupe)
	dst.NormalizationFailed.Merge(src.NormalizationFailed)
	dst.Errors.Merge(src.Errors)
}

// MergeIOCMetrics merges src into dst, preserving sample limits.
func MergeIOCMetrics(dst *IOCMetrics, src IOCMetrics) {
	if dst == nil {
		return
	}
	dst.Matches.Merge(src.Matches)
	dst.MatchesDryRun.Merge(src.MatchesDryRun)
	dst.Alerts.Merge(src.Alerts)
	dst.AlertsMuted.Merge(src.AlertsMuted)
}

// AppendUniqueLower appends a lower-cased value when not already present.
func AppendUniqueLower(list *[]string, value string) {
	if list == nil {
		return
	}
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return
	}
	for _, existing := range *list {
		if existing == v {
			return
		}
	}
	*list = append(*list, v)
}
