package rules

import "testing"

func TestEnqueueJobRequestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		req     EnqueueJobRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: EnqueueJobRequest{
				EventIDs: []string{"1", "2"},
				SiteID:   "site",
				Scope:    "scope",
				Priority: 10,
			},
		},
		{
			name: "missing event ids",
			req: EnqueueJobRequest{
				SiteID: "site",
				Scope:  "scope",
			},
			wantErr: true,
		},
		{
			name: "missing site id",
			req: EnqueueJobRequest{
				EventIDs: []string{"1"},
				Scope:    "scope",
			},
			wantErr: true,
		},
		{
			name: "missing scope",
			req: EnqueueJobRequest{
				EventIDs: []string{"1"},
				SiteID:   "site",
			},
			wantErr: true,
		},
		{
			name: "priority too low",
			req: EnqueueJobRequest{
				EventIDs: []string{"1"},
				SiteID:   "site",
				Scope:    "scope",
				Priority: -1,
			},
			wantErr: true,
		},
		{
			name: "priority too high",
			req: EnqueueJobRequest{
				EventIDs: []string{"1"},
				SiteID:   "site",
				Scope:    "scope",
				Priority: 101,
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.req.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
