package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEventHandlers_BulkInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventRepo := mocks.NewMockEventRepository(ctrl)
	eventSvc := service.MustNewEventService(service.EventServiceOptions{
		Repo: mockEventRepo,
		Config: service.EventServiceConfig{
			MaxBatch:                 100,
			ThreatScoreProcessCutoff: 0.7,
		},
	})
	handlers := &EventHandlers{
		Svc:    eventSvc,
		Filter: service.NewEventFilterService(),
	}

	tests := []struct {
		name           string
		requestBody    PuppeteerEventBatch
		mockSetup      func()
		expectedStatus int
		expectedCount  int
	}{
		{
			name: "successful bulk insert",
			requestBody: PuppeteerEventBatch{
				BatchID:   "batch-123",
				SessionID: "session-456",
				Events: []PuppeteerEvent{
					{
						ID:     "event-1",
						Method: "Network.requestWillBeSent",
						Params: PuppeteerEventParams{
							Timestamp: time.Now().UnixMilli(),
							SessionID: "session-456",
							Attribution: PuppeteerAttribution{
								URL:       "https://example.com",
								UserAgent: "test-agent",
							},
							Payload: map[string]any{
								"url":    "https://example.com/api",
								"method": "GET",
							},
						},
						Metadata: PuppeteerEventMetadata{
							Category: "network",
							Tags:     []string{"network", "request"},
							ProcessingHints: map[string]any{
								"isHighPriority": false,
							},
							SequenceNumber: 1,
						},
					},
				},
				BatchMetadata: PuppeteerBatchMetadata{
					CreatedAt:  time.Now().UnixMilli(),
					EventCount: 1,
					TotalSize:  1024,
				},
				SequenceInfo: PuppeteerSequenceInfo{
					SequenceNumber: 1,
					IsFirstBatch:   true,
					IsLastBatch:    true,
				},
			},
			mockSetup: func() {
				mockEventRepo.EXPECT().
					BulkInsertWithProcessingFlags(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(1, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name: "high priority event",
			requestBody: PuppeteerEventBatch{
				BatchID:   "batch-789",
				SessionID: "session-456",
				Events: []PuppeteerEvent{
					{
						ID:     "event-2",
						Method: "Security.securityStateChanged",
						Params: PuppeteerEventParams{
							Timestamp: time.Now().UnixMilli(),
							SessionID: "session-456",
							Attribution: PuppeteerAttribution{
								URL: "https://malicious.com",
							},
							Payload: map[string]any{
								"securityState": "insecure",
							},
						},
						Metadata: PuppeteerEventMetadata{
							Category: "security",
							Tags:     []string{"security", "threat"},
							ProcessingHints: map[string]any{
								"isHighPriority": true,
							},
							SequenceNumber: 1,
						},
					},
				},
				BatchMetadata: PuppeteerBatchMetadata{
					CreatedAt:  time.Now().UnixMilli(),
					EventCount: 1,
					TotalSize:  512,
				},
			},
			mockSetup: func() {
				mockEventRepo.EXPECT().
					BulkInsertWithProcessingFlags(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(1, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/events/bulk", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handlers.BulkInsert(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.InDelta(t, float64(tt.expectedCount), response["inserted"], 1e-9)
				assert.Equal(t, tt.requestBody.BatchID, response["batch_id"])
				assert.Equal(t, tt.requestBody.SessionID, response["session_id"])
			}
		})
	}
}

func TestEventHandlers_BulkInsert_JobIdLinkage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventRepo := mocks.NewMockEventRepository(ctrl)
	eventSvc := service.MustNewEventService(service.EventServiceOptions{
		Repo: mockEventRepo,
		Config: service.EventServiceConfig{
			MaxBatch:                 100,
			ThreatScoreProcessCutoff: 0.7,
		},
	})
	handlers := &EventHandlers{
		Svc:    eventSvc,
		Filter: service.NewEventFilterService(),
	}

	tests := []struct {
		name           string
		jobID          string
		expectedStatus int
		expectJobID    bool
	}{
		{
			name:           "valid jobId linkage",
			jobID:          "550e8400-e29b-41d4-a716-446655440000",
			expectedStatus: http.StatusOK,
			expectJobID:    true,
		},
		{
			name:           "missing jobId (allowed)",
			jobID:          "",
			expectedStatus: http.StatusOK,
			expectJobID:    false,
		},
		{
			name:           "invalid jobId format",
			jobID:          "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectJobID:    false,
		},
		{
			name:           "malformed jobId",
			jobID:          "550e8400-e29b-41d4-a716-44665544000", // missing digit
			expectedStatus: http.StatusBadRequest,
			expectJobID:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedStatus == http.StatusOK {
				mockEventRepo.EXPECT().
					BulkInsertWithProcessingFlags(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_, req, _ any) (int, error) {
						bulkReq := req.(model.BulkEventRequest)
						if tt.expectJobID {
							assert.NotNil(t, bulkReq.SourceJobID)
							assert.Equal(t, tt.jobID, *bulkReq.SourceJobID)
						} else {
							assert.Nil(t, bulkReq.SourceJobID)
						}
						return 1, nil
					})
			}

			batch := PuppeteerEventBatch{
				BatchID:   "batch-123",
				SessionID: "session-456",
				Events: []PuppeteerEvent{
					{
						ID:     "event-1",
						Method: "Network.requestWillBeSent",
						Params: PuppeteerEventParams{
							Timestamp: time.Now().UnixMilli(),
							SessionID: "session-456",
							Payload: map[string]any{
								"url": "https://example.com",
							},
						},
						Metadata: PuppeteerEventMetadata{
							Category:       "network",
							Tags:           []string{"network"},
							SequenceNumber: 1,
						},
					},
				},
				BatchMetadata: PuppeteerBatchMetadata{
					CreatedAt:  time.Now().UnixMilli(),
					EventCount: 1,
					TotalSize:  100,
					JobID:      tt.jobID,
				},
			}

			body, err := json.Marshal(batch)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/events/bulk", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handlers.BulkInsert(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestEventHandlers_transformPuppeteerBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventRepo := mocks.NewMockEventRepository(ctrl)
	handlers := &EventHandlers{
		Svc: service.MustNewEventService(service.EventServiceOptions{
			Repo: mockEventRepo,
			Config: service.EventServiceConfig{
				MaxBatch:                 100,
				ThreatScoreProcessCutoff: 0.7,
			},
		}),
	}

	batch := PuppeteerEventBatch{
		SessionID: "session-123",
		Events: []PuppeteerEvent{
			{
				Method: "Network.responseReceived",
				Params: PuppeteerEventParams{
					Timestamp: 1640995200000, // 2022-01-01 00:00:00 UTC in milliseconds
					Payload: map[string]any{
						"url":    "https://example.com",
						"status": 200,
					},
				},
				Metadata: PuppeteerEventMetadata{
					Category: "network",
					Tags:     []string{"network", "response"},
					ProcessingHints: map[string]any{
						"isHighPriority": true,
					},
					SequenceNumber: 1,
				},
			},
		},
	}

	result, err := handlers.transformPuppeteerBatch(&batch)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "session-123", result.SessionID)
	assert.Len(t, result.Events, 1)

	event := result.Events[0]
	assert.Equal(t, "Network.responseReceived", event.Type)
	assert.NotNil(t, event.Priority)
	assert.Equal(t, 75, *event.Priority) // High priority
	assert.Equal(t, time.Unix(0, 1640995200000*int64(time.Millisecond)), event.Timestamp)

	// Check that payload was marshaled correctly
	var payload map[string]any
	err = json.Unmarshal(event.Data, &payload)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", payload["url"])
	assert.InDelta(t, float64(200), payload["status"], 1e-9)
}

func TestEventHandlers_transformPuppeteerBatch_JobIdLinkage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventRepo := mocks.NewMockEventRepository(ctrl)
	handlers := &EventHandlers{
		Svc: service.MustNewEventService(service.EventServiceOptions{
			Repo: mockEventRepo,
			Config: service.EventServiceConfig{
				MaxBatch:                 100,
				ThreatScoreProcessCutoff: 0.7,
			},
		}),
	}

	tests := []struct {
		name        string
		jobID       string
		expectJobID bool
	}{
		{
			name:        "with valid jobId",
			jobID:       "550e8400-e29b-41d4-a716-446655440000",
			expectJobID: true,
		},
		{
			name:        "with empty jobId",
			jobID:       "",
			expectJobID: false,
		},
		{
			name:        "without jobId field",
			jobID:       "",
			expectJobID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := PuppeteerEventBatch{
				SessionID: "session-123",
				Events: []PuppeteerEvent{
					{
						Method: "Network.responseReceived",
						Params: PuppeteerEventParams{
							Timestamp: 1640995200000,
							Payload: map[string]any{
								"url": "https://example.com",
							},
						},
						Metadata: PuppeteerEventMetadata{
							Category:       "network",
							Tags:           []string{"network"},
							SequenceNumber: 1,
						},
					},
				},
				BatchMetadata: PuppeteerBatchMetadata{
					CreatedAt:  time.Now().UnixMilli(),
					EventCount: 1,
					TotalSize:  100,
					JobID:      tt.jobID,
				},
			}

			result, err := handlers.transformPuppeteerBatch(&batch)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, "session-123", result.SessionID)
			assert.Len(t, result.Events, 1)

			if tt.expectJobID {
				assert.NotNil(t, result.SourceJobID)
				assert.Equal(t, tt.jobID, *result.SourceJobID)
			} else {
				assert.Nil(t, result.SourceJobID)
			}
		})
	}
}

func TestEventHandlers_ListByJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventRepo := mocks.NewMockEventRepository(ctrl)
	eventSvc := service.MustNewEventService(service.EventServiceOptions{
		Repo: mockEventRepo,
		Config: service.EventServiceConfig{
			MaxBatch:                 100,
			ThreatScoreProcessCutoff: 0.7,
		},
	})
	h := &EventHandlers{
		Svc:    eventSvc,
		Filter: service.NewEventFilterService(),
	}

	// Missing id
	r := httptest.NewRequest(http.MethodGet, "/api/jobs//events", nil)
	w := httptest.NewRecorder()
	h.ListByJob(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Success
	expected := []*model.Event{{ID: "e1"}, {ID: "e2"}}
	jobID := "550e8400-e29b-41d4-a716-446655440000"
	mockEventRepo.EXPECT().
		ListByJob(gomock.Any(), model.EventListByJobOptions{JobID: jobID, Limit: 25, Offset: 5}).
		Return(&model.EventListPage{Events: expected}, nil)
	r2 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/events?limit=25&offset=5", nil)
	r2.SetPathValue("id", jobID)
	w2 := httptest.NewRecorder()
	h.ListByJob(w2, r2)
	assert.Equal(t, http.StatusOK, w2.Code)
	var got eventListResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &got))
	assert.Len(t, got.Events, 2)
	assert.Equal(t, "e1", got.Events[0].ID)

	// Cursor-based pagination uses keyset tokens and resets offset
	next := "next-token"
	prev := "prev-token"
	cursor := "cursor-123"
	expectedOpts := model.EventListByJobOptions{
		JobID:        jobID,
		Limit:        50,
		Offset:       0,
		CursorBefore: &cursor,
	}
	mockEventRepo.EXPECT().
		ListByJob(gomock.Any(), expectedOpts).
		Return(&model.EventListPage{Events: expected, NextCursor: &next, PrevCursor: &prev}, nil)
	r3 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/events?cursor="+cursor+"&dir=prev", nil)
	r3.SetPathValue("id", jobID)
	w3 := httptest.NewRecorder()
	h.ListByJob(w3, r3)
	assert.Equal(t, http.StatusOK, w3.Code)

	var gotCursor eventListResponse
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &gotCursor))
	assert.Equal(t, &next, gotCursor.NextCursor)
	assert.Equal(t, &prev, gotCursor.PrevCursor)

	// Forward cursor defaults (dir empty/after/next) and trims whitespace
	forwardCursor := "cursor-fwd"
	trimmedCursor := "   " + forwardCursor + "   "
	next2 := "next-token-2"
	mockEventRepo.EXPECT().
		ListByJob(gomock.Any(), model.EventListByJobOptions{
			JobID:       jobID,
			Limit:       50,
			Offset:      0,
			CursorAfter: &forwardCursor,
		}).
		Return(&model.EventListPage{Events: expected, NextCursor: &next2}, nil)
	encodedForward := "/api/jobs/" + jobID + "/events?" + url.Values{
		"cursor": []string{trimmedCursor},
		"dir":    []string{" after "},
	}.Encode()
	rForward := httptest.NewRequest(http.MethodGet, encodedForward, nil)
	rForward.SetPathValue("id", jobID)
	wForward := httptest.NewRecorder()
	h.ListByJob(wForward, rForward)
	assert.Equal(t, http.StatusOK, wForward.Code)

	var gotForward eventListResponse
	require.NoError(t, json.Unmarshal(wForward.Body.Bytes(), &gotForward))
	assert.Equal(t, &next2, gotForward.NextCursor)

	// Invalid dir should return 400
	r4 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/events?cursor="+cursor+"&dir=perv", nil)
	r4.SetPathValue("id", jobID)
	w4 := httptest.NewRecorder()
	h.ListByJob(w4, r4)
	assert.Equal(t, http.StatusBadRequest, w4.Code)
}

func TestEventHandlers_BulkInsert_PersistsAll_ProcessesOnlyNetwork(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventRepo := mocks.NewMockEventRepository(ctrl)
	eventSvc := service.MustNewEventService(service.EventServiceOptions{
		Repo: mockEventRepo,
		Config: service.EventServiceConfig{
			MaxBatch:                 100,
			ThreatScoreProcessCutoff: 0.7,
		},
	})
	handlers := &EventHandlers{Svc: eventSvc, Filter: service.NewEventFilterService()}

	batch := PuppeteerEventBatch{
		BatchID:   "batch-xyz",
		SessionID: "session-abc",
		Events: []PuppeteerEvent{
			{
				ID:     "e1",
				Method: "Network.requestWillBeSent",
				Params: PuppeteerEventParams{
					Timestamp: time.Now().UnixMilli(),
					SessionID: "session-abc",
					Payload:   map[string]any{"url": "https://ex.com"},
				},
			},
			{
				ID:     "e2",
				Method: "Runtime.consoleAPICalled",
				Params: PuppeteerEventParams{
					Timestamp: time.Now().UnixMilli(),
					SessionID: "session-abc",
					Payload:   map[string]any{"type": "log"},
				},
			},
		},
		BatchMetadata: PuppeteerBatchMetadata{
			CreatedAt:  time.Now().UnixMilli(),
			EventCount: 2,
			TotalSize:  10,
		},
	}

	mockEventRepo.EXPECT().
		BulkInsertWithProcessingFlags(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(map[int]bool{})).
		DoAndReturn(func(_, _ any, m map[int]bool) (int, error) {
			// Expect only the first (network) event to be processable
			assert.True(t, m[0])
			assert.False(t, m[1])
			return 2, nil
		})

	body, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/api/events/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.BulkInsert(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
