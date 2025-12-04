package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func BenchmarkEventRepo_BulkInsert(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Create test events
		events := make([]model.RawEvent, 100)
		for i := range events {
			data := fmt.Sprintf(`{"index":%d}`, i)
			events[i] = model.RawEvent{
				Type:     "benchmark_event",
				Data:     json.RawMessage(data),
				Priority: intPtr(1),
			}
		}

		req := model.BulkEventRequest{
			SessionID: "550e8400-e29b-41d4-a716-446655440005",
			Events:    events,
		}

		b.ResetTimer()
		for b.Loop() {
			_, err := eventRepo.BulkInsert(ctx, req, false)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkEventRepo_BulkInsertCopy(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Create test events
		events := make([]model.RawEvent, 100)
		for i := range 100 {
			data := fmt.Sprintf(`{"index":%d}`, i)
			events[i] = model.RawEvent{
				Type:     "benchmark_event_copy",
				Data:     json.RawMessage(data),
				Priority: intPtr(1),
			}
		}

		req := model.BulkEventRequest{
			SessionID: "550e8400-e29b-41d4-a716-446655440006",
			Events:    events,
		}

		b.ResetTimer()
		for b.Loop() {
			_, err := eventRepo.BulkInsertCopy(ctx, req, false)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
