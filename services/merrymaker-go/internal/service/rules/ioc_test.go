package rules

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

type stubIOCCache struct {
	hosts map[string]*model.IOC
}

func (s stubIOCCache) LookupHost(_ context.Context, host string) (*model.IOC, error) {
	if ioc, ok := s.hosts[strings.ToLower(strings.TrimSpace(host))]; ok {
		return ioc, nil
	}
	return nil, ErrNotFound
}

func TestIOCEvaluator_AlertContextIncludesAttribution(t *testing.T) {
	ctx := context.Background()
	ioc := &model.IOC{
		ID:        "ioc-1",
		Type:      model.IOCTypeFQDN,
		Value:     "example.com",
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	fa := &fakeAlertRepo{}
	eval := &IOCEvaluator{
		Caches: Caches{
			IOCs: stubIOCCache{hosts: map[string]*model.IOC{
				"example.com": ioc,
			}},
		},
		Alerter:  &core.AlertService{Repo: fa},
		AlertTTL: 0,
	}

	req := IOCRequest{
		Scope:      ScopeKey{SiteID: "site-1", Scope: "default"},
		Host:       "example.com",
		SiteID:     "site-1",
		JobID:      "job-1",
		RequestURL: "https://example.com/api",
		PageURL:    "https://example.com/page",
		Referrer:   "https://example.com/",
		UserAgent:  "UA",
		EventID:    "evt-1",
	}

	alerted, err := eval.Evaluate(ctx, req)
	require.NoError(t, err)
	require.True(t, alerted)
	require.Len(t, fa.created, 1)

	var ctxMap map[string]any
	require.NoError(t, json.Unmarshal(fa.created[0].EventContext, &ctxMap))

	assert.Equal(t, "job-1", ctxMap["job_id"])
	assert.Equal(t, "evt-1", ctxMap["event_id"])
	assert.Equal(t, "https://example.com/api", ctxMap["request_url"])
	assert.Equal(t, "https://example.com/page", ctxMap["page_url"])
	assert.Equal(t, "https://example.com/", ctxMap["referrer"])
	assert.Equal(t, "UA", ctxMap["user_agent"])
	assert.Equal(t, "ioc-1", ctxMap["ioc_id"])
}
