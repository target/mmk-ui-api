package data

import (
	"reflect"
	"testing"

	"github.com/target/mmk-ui-api/internal/core"
)

var (
	_ core.JobRepository   = (*JobRepo)(nil)
	_ core.JobRepositoryTx = (*JobRepo)(nil)
)

func TestJobRepoExportedMethodsMatchAllowlist(t *testing.T) {
	allowed := map[string]struct{}{
		"Complete":                      {},
		"CountAggregatesBySources":      {},
		"CountBrowserBySource":          {},
		"CountBySource":                 {},
		"Create":                        {},
		"CreateInTx":                    {},
		"Delete":                        {},
		"DeleteByPayloadField":          {},
		"DeleteOldJobResults":           {},
		"DeleteOldJobs":                 {},
		"Fail":                          {},
		"FailStalePendingJobs":          {},
		"GetByID":                       {},
		"Heartbeat":                     {},
		"JobStatesByTaskName":           {},
		"List":                          {},
		"ListBySiteWithFilters":         {},
		"ListBySource":                  {},
		"ListRecentByType":              {},
		"ListRecentByTypeWithSiteNames": {},
		"ReserveNext":                   {},
		"RunningJobExistsByTaskName":    {},
		"Stats":                         {},
		"WaitForNotification":           {},
	}

	methods := reflect.TypeOf(&JobRepo{})
	seen := make(map[string]struct{})

	for i := range methods.NumMethod() {
		m := methods.Method(i)
		if !m.IsExported() {
			continue
		}
		name := m.Name
		if _, ok := allowed[name]; !ok {
			t.Fatalf("unexpected exported method on JobRepo: %s", name)
		}
		seen[name] = struct{}{}
	}

	for name := range allowed {
		if _, ok := seen[name]; !ok {
			t.Fatalf("expected JobRepo to export method %s", name)
		}
	}
}
