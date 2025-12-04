package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// BenchmarkJobRepo_Create benchmarks job creation performance.
func BenchmarkJobRepo_Create(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		req := &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com", "data": "benchmark test"}`),
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := repo.Create(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkJobRepo_ReserveNext benchmarks job reservation performance.
func BenchmarkJobRepo_ReserveNext(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Pre-populate with jobs
		const numJobs = 1000
		for i := range numJobs {
			req := &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(fmt.Sprintf(`{"url": "https://example%d.com"}`, i)),
				Priority: i % 100, // Vary priorities
			}
			_, err := repo.Create(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for b.Loop() {
			_, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
			if err != nil && !errors.Is(err, model.ErrNoJobsAvailable) {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkJobRepo_ConcurrentReserveNext benchmarks concurrent job reservation.
func BenchmarkJobRepo_ConcurrentReserveNext(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Pre-populate with jobs
		const numJobs = 10000
		for i := range numJobs {
			req := &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(fmt.Sprintf(`{"url": "https://example%d.com"}`, i)),
				Priority: i % 100,
			}
			_, err := repo.Create(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
				if err != nil && !errors.Is(err, model.ErrNoJobsAvailable) {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkJobRepo_Complete benchmarks job completion performance.
func BenchmarkJobRepo_Complete(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Pre-populate and reserve jobs
		var jobIDs []string
		for i := 0; b.Loop(); i++ {
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(fmt.Sprintf(`{"url": "https://example%d.com"}`, i)),
			}
			_, err := repo.Create(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}

			reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
			if err != nil {
				b.Fatal(err)
			}
			jobIDs = append(jobIDs, reserved.ID)
		}

		b.ResetTimer()
		for i := range b.N {
			_, err := repo.Complete(context.Background(), jobIDs[i])
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkJobRepo_Heartbeat benchmarks heartbeat performance.
func BenchmarkJobRepo_Heartbeat(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Pre-populate and reserve jobs
		var jobIDs []string
		for i := 0; b.Loop(); i++ {
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(fmt.Sprintf(`{"url": "https://example%d.com"}`, i)),
			}
			_, err := repo.Create(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}

			reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
			if err != nil {
				b.Fatal(err)
			}
			jobIDs = append(jobIDs, reserved.ID)
		}

		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			_, err := repo.Heartbeat(context.Background(), jobIDs[i], 60)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkJobRepo_Stats benchmarks statistics query performance.
func BenchmarkJobRepo_Stats(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Pre-populate with jobs in various states
		const numJobs = 1000
		for i := range numJobs {
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(fmt.Sprintf(`{"url": "https://example%d.com"}`, i)),
			}
			job, err := repo.Create(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}

			// Reserve and complete some jobs to create varied states
			if i%4 != 0 {
				continue
			}

			_, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
			if err != nil {
				b.Fatal(err)
			}

			if i%8 != 0 {
				continue
			}

			_, err = repo.Complete(context.Background(), job.ID)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for b.Loop() {
			_, err := repo.Stats(context.Background(), model.JobTypeBrowser)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkJobRepo_MultiWorkerScenario benchmarks a realistic multi-worker scenario.
func BenchmarkJobRepo_MultiWorkerScenario(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithAutoDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		const numWorkers = 10
		const jobsPerWorker = 100

		// Pre-populate with jobs
		for i := range numWorkers * jobsPerWorker {
			req := &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(fmt.Sprintf(`{"url": "https://example%d.com"}`, i)),
				Priority: i % 100,
			}
			_, err := repo.Create(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()

		var wg sync.WaitGroup
		for range numWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range jobsPerWorker {
					// Reserve job
					job, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
					if err != nil {
						if !errors.Is(err, model.ErrNoJobsAvailable) {
							b.Error(err)
						}
						continue
					}

					// Simulate work with heartbeat
					_, err = repo.Heartbeat(context.Background(), job.ID, 60)
					if err != nil {
						b.Error(err)
						continue
					}

					// Complete job
					_, err = repo.Complete(context.Background(), job.ID)
					if err != nil {
						b.Error(err)
					}
				}
			}()
		}
		wg.Wait()
	})
}

// BenchmarkJobRepo_CreateAndReserveRace benchmarks race conditions between create and reserve.
func BenchmarkJobRepo_CreateAndReserveRace(b *testing.B) {
	testutil.SkipIfNoTestDB(b)

	testutil.WithTestDB(b, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		b.ResetTimer()

		var wg sync.WaitGroup

		// Creator goroutines
		for i := range 5 {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := range b.N / 5 {
					req := &model.CreateJobRequest{
						Type: model.JobTypeBrowser,
						Payload: json.RawMessage(
							fmt.Sprintf(`{"worker": %d, "job": %d}`, workerID, j),
						),
					}
					_, err := repo.Create(context.Background(), req)
					if err != nil {
						b.Error(err)
					}
				}
			}(i)
		}

		// Consumer goroutines
		for range 3 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(1 * time.Millisecond)
				defer ticker.Stop()

				for {
					_, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
					if err != nil {
						if errors.Is(err, model.ErrNoJobsAvailable) {
							<-ticker.C
							continue
						}
						b.Error(err)
						continue
					}
				}
			}()
		}

		wg.Wait()
	})
}
