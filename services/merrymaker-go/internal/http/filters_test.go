package httpx

import (
	"net/url"
	"testing"
)

func TestParseSortParam_CombinedFormat(t *testing.T) {
	tests := []struct {
		name          string
		sortValue     string
		expectedField string
		expectedDir   string
	}{
		{
			name:          "valid asc",
			sortValue:     "created_at:asc",
			expectedField: "created_at",
			expectedDir:   "asc",
		},
		{
			name:          "valid desc",
			sortValue:     "name:desc",
			expectedField: "name",
			expectedDir:   "desc",
		},
		{
			name:          "uppercase direction",
			sortValue:     "id:DESC",
			expectedField: "id",
			expectedDir:   "desc",
		},
		{
			name:          "mixed case direction",
			sortValue:     "email:AsC",
			expectedField: "email",
			expectedDir:   "asc",
		},
		{
			name:          "invalid direction",
			sortValue:     "status:invalid",
			expectedField: "status",
			expectedDir:   "",
		},
		{
			name:          "empty direction",
			sortValue:     "age:",
			expectedField: "age",
			expectedDir:   "",
		},
		{
			name:          "whitespace around parts",
			sortValue:     " created_at : desc ",
			expectedField: "created_at",
			expectedDir:   "desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("sort", tt.sortValue)

			field, dir := ParseSortParam(q, "sort", "dir")

			if field != tt.expectedField {
				t.Errorf("Expected field %q, got %q", tt.expectedField, field)
			}
			if dir != tt.expectedDir {
				t.Errorf("Expected dir %q, got %q", tt.expectedDir, dir)
			}
		})
	}
}

func TestParseSortParam_SeparateFormat(t *testing.T) {
	tests := []struct {
		name          string
		sortValue     string
		dirValue      string
		expectedField string
		expectedDir   string
	}{
		{
			name:          "valid asc",
			sortValue:     "created_at",
			dirValue:      "asc",
			expectedField: "created_at",
			expectedDir:   "asc",
		},
		{
			name:          "valid desc",
			sortValue:     "name",
			dirValue:      "desc",
			expectedField: "name",
			expectedDir:   "desc",
		},
		{
			name:          "uppercase direction",
			sortValue:     "id",
			dirValue:      "DESC",
			expectedField: "id",
			expectedDir:   "desc",
		},
		{
			name:          "invalid direction",
			sortValue:     "status",
			dirValue:      "invalid",
			expectedField: "status",
			expectedDir:   "",
		},
		{
			name:          "empty direction",
			sortValue:     "age",
			dirValue:      "",
			expectedField: "age",
			expectedDir:   "",
		},
		{
			name:          "whitespace in values",
			sortValue:     " email ",
			dirValue:      " asc ",
			expectedField: "email",
			expectedDir:   "asc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("sort", tt.sortValue)
			q.Set("dir", tt.dirValue)

			field, dir := ParseSortParam(q, "sort", "dir")

			if field != tt.expectedField {
				t.Errorf("Expected field %q, got %q", tt.expectedField, field)
			}
			if dir != tt.expectedDir {
				t.Errorf("Expected dir %q, got %q", tt.expectedDir, dir)
			}
		})
	}
}

func TestParseSortParam_CombinedTakesPrecedence(t *testing.T) {
	// When both combined and separate formats are present, combined should take precedence
	q := url.Values{}
	q.Set("sort", "name:desc")
	q.Set("dir", "asc") // This should be ignored

	field, dir := ParseSortParam(q, "sort", "dir")

	if field != "name" {
		t.Errorf("Expected field %q, got %q", "name", field)
	}
	if dir != "desc" {
		t.Errorf("Expected dir %q, got %q", "desc", dir)
	}
}

func TestParseSortParam_EmptyValues(t *testing.T) {
	q := url.Values{}

	field, dir := ParseSortParam(q, "sort", "dir")

	if field != "" {
		t.Errorf("Expected empty field, got %q", field)
	}
	if dir != "" {
		t.Errorf("Expected empty dir, got %q", dir)
	}
}

func TestParseSortParam_CustomKeys(t *testing.T) {
	q := url.Values{}
	q.Set("order_by", "created_at")
	q.Set("order_dir", "desc")

	field, dir := ParseSortParam(q, "order_by", "order_dir")

	if field != "created_at" {
		t.Errorf("Expected field %q, got %q", "created_at", field)
	}
	if dir != "desc" {
		t.Errorf("Expected dir %q, got %q", "desc", dir)
	}
}

func TestParseSortParam_MultipleColons(t *testing.T) {
	// Only the first colon should be used for splitting
	q := url.Values{}
	q.Set("sort", "table:column:desc")

	field, dir := ParseSortParam(q, "sort", "dir")

	// Should split on first colon only
	if field != "table" {
		t.Errorf("Expected field %q, got %q", "table", field)
	}
	// "column:desc" is not a valid direction
	if dir != "" {
		t.Errorf("Expected empty dir, got %q", dir)
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are defined correctly
	const (
		expectedTrue  = "true"
		expectedFalse = "false"
		expectedAsc   = "asc"
		expectedDesc  = "desc"
	)

	if StrTrue != expectedTrue {
		t.Errorf("Expected StrTrue to be %q, got %q", expectedTrue, StrTrue)
	}
	if StrFalse != expectedFalse {
		t.Errorf("Expected StrFalse to be %q, got %q", expectedFalse, StrFalse)
	}
	if SortDirAsc != expectedAsc {
		t.Errorf("Expected SortDirAsc to be %q, got %q", expectedAsc, SortDirAsc)
	}
	if SortDirDesc != expectedDesc {
		t.Errorf("Expected SortDirDesc to be %q, got %q", expectedDesc, SortDirDesc)
	}
}
