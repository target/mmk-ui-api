package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTemplateData(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	meta := PageMeta{
		Title:       "Test Title",
		PageTitle:   "Test Page",
		CurrentPage: "test",
	}

	builder := NewTemplateData(r, meta)
	data := builder.Build()

	// Check basePageData fields
	if data["Title"] != "Test Title" {
		t.Errorf("Title = %v, want %v", data["Title"], "Test Title")
	}
	if data["PageTitle"] != "Test Page" {
		t.Errorf("PageTitle = %v, want %v", data["PageTitle"], "Test Page")
	}
	if data["CurrentPage"] != "test" {
		t.Errorf("CurrentPage = %v, want %v", data["CurrentPage"], "test")
	}
	if data["IsAuthenticated"] != false {
		t.Errorf("IsAuthenticated = %v, want %v", data["IsAuthenticated"], false)
	}
	if data["CanManageAllowlist"] != false {
		t.Errorf("CanManageAllowlist = %v, want %v", data["CanManageAllowlist"], false)
	}
}

func TestTemplateDataBuilder_WithPagination_PrevAndNext(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/secrets?q=test", nil)
	meta := PageMeta{Title: "Secrets", PageTitle: "Secrets", CurrentPage: "secrets"}

	data := NewTemplateData(r, meta).
		WithPagination(PaginationData{
			Page:       2,
			PageSize:   20,
			HasPrev:    true,
			HasNext:    true,
			StartIndex: 21,
			EndIndex:   40,
			BasePath:   "/secrets",
		}).
		Build()

	// Verify pagination fields
	assertIntField(t, intFieldAssertion{Data: data, Key: "Page", Want: 2})
	assertIntField(t, intFieldAssertion{Data: data, Key: "PageSize", Want: 20})
	assertBoolField(t, boolFieldAssertion{Data: data, Key: "HasPrev", Want: true})
	assertBoolField(t, boolFieldAssertion{Data: data, Key: "HasNext", Want: true})
	assertIntField(t, intFieldAssertion{Data: data, Key: "StartIndex", Want: 21})
	assertIntField(t, intFieldAssertion{Data: data, Key: "EndIndex", Want: 40})

	// Check PrevURL and NextURL are set and non-empty
	assertStringFieldExists(t, data, "PrevURL")
	assertStringFieldExists(t, data, "NextURL")
}

type intFieldAssertion struct {
	Data map[string]any
	Key  string
	Want int
}

func assertIntField(t *testing.T, params intFieldAssertion) {
	t.Helper()
	if got, ok := params.Data[params.Key].(int); !ok || got != params.Want {
		t.Errorf("%s = %v, want %v", params.Key, params.Data[params.Key], params.Want)
	}
}

type boolFieldAssertion struct {
	Data map[string]any
	Key  string
	Want bool
}

func assertBoolField(t *testing.T, params boolFieldAssertion) {
	t.Helper()
	if got, ok := params.Data[params.Key].(bool); !ok || got != params.Want {
		t.Errorf("%s = %v, want %v", params.Key, params.Data[params.Key], params.Want)
	}
}

func assertStringFieldExists(t *testing.T, data map[string]any, key string) {
	t.Helper()
	val, ok := data[key]
	if !ok {
		t.Errorf("%s not set", key)
		return
	}
	if str, ok := val.(string); !ok || str == "" {
		t.Errorf("%s is empty or not a string", key)
	}
}

func TestTemplateDataBuilder_WithPagination_NoPrev(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/secrets", nil)
	meta := PageMeta{Title: "Secrets", PageTitle: "Secrets", CurrentPage: "secrets"}

	data := NewTemplateData(r, meta).
		WithPagination(PaginationData{
			Page:       1,
			PageSize:   20,
			HasPrev:    false,
			HasNext:    true,
			StartIndex: 1,
			EndIndex:   20,
			BasePath:   "/secrets",
		}).
		Build()

	if _, ok := data["PrevURL"]; ok {
		t.Error("PrevURL should not be set when HasPrev is false")
	}
	if _, ok := data["NextURL"]; !ok {
		t.Error("NextURL should be set when HasNext is true")
	}
}

func TestTemplateDataBuilder_WithPagination_NoNext(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/secrets", nil)
	meta := PageMeta{Title: "Secrets", PageTitle: "Secrets", CurrentPage: "secrets"}

	data := NewTemplateData(r, meta).
		WithPagination(PaginationData{
			Page:       3,
			PageSize:   20,
			HasPrev:    true,
			HasNext:    false,
			StartIndex: 41,
			EndIndex:   50,
			BasePath:   "/secrets",
		}).
		Build()

	if _, ok := data["PrevURL"]; !ok {
		t.Error("PrevURL should be set when HasPrev is true")
	}
	if _, ok := data["NextURL"]; ok {
		t.Error("NextURL should not be set when HasNext is false")
	}
}

func TestTemplateDataBuilder_WithError(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	meta := PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"}

	data := NewTemplateData(r, meta).
		WithError("Something went wrong").
		Build()

	if data["Error"] != true {
		t.Errorf("Error = %v, want %v", data["Error"], true)
	}
	if data["ErrorMessage"] != "Something went wrong" {
		t.Errorf("ErrorMessage = %v, want %v", data["ErrorMessage"], "Something went wrong")
	}
}

func TestTemplateDataBuilder_WithFieldErrors(t *testing.T) {
	t.Run("with errors", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		meta := PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"}

		errs := map[string]string{
			"name":  "Name is required",
			"email": "Email is invalid",
		}

		data := NewTemplateData(r, meta).
			WithFieldErrors(errs).
			Build()

		if _, ok := data["Errors"]; !ok {
			t.Error("Errors should be set when errors are provided")
		}

		errors, ok := data["Errors"].(map[string]string)
		if !ok {
			t.Fatal("Errors is not a map[string]string")
		}

		if errors["name"] != "Name is required" {
			t.Errorf("Errors[name] = %v, want %v", errors["name"], "Name is required")
		}
		if errors["email"] != "Email is invalid" {
			t.Errorf("Errors[email] = %v, want %v", errors["email"], "Email is invalid")
		}
	})

	t.Run("with empty errors", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		meta := PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"}

		data := NewTemplateData(r, meta).
			WithFieldErrors(map[string]string{}).
			Build()

		if _, ok := data["Errors"]; ok {
			t.Error("Errors should not be set when errors map is empty")
		}
	})

	t.Run("with nil errors", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		meta := PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"}

		data := NewTemplateData(r, meta).
			WithFieldErrors(nil).
			Build()

		if _, ok := data["Errors"]; ok {
			t.Error("Errors should not be set when errors map is nil")
		}
	})
}

func TestTemplateDataBuilder_With(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	meta := PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"}

	data := NewTemplateData(r, meta).
		With("CustomField", "CustomValue").
		With("Count", 42).
		Build()

	if data["CustomField"] != "CustomValue" {
		t.Errorf("CustomField = %v, want %v", data["CustomField"], "CustomValue")
	}
	if data["Count"] != 42 {
		t.Errorf("Count = %v, want %v", data["Count"], 42)
	}
}

func TestTemplateDataBuilder_Chaining(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/secrets?q=test", nil)
	meta := PageMeta{Title: "Secrets", PageTitle: "Secrets", CurrentPage: "secrets"}

	data := NewTemplateData(r, meta).
		WithPagination(PaginationData{
			Page:       1,
			PageSize:   20,
			HasPrev:    false,
			HasNext:    true,
			StartIndex: 1,
			EndIndex:   20,
			BasePath:   "/secrets",
		}).
		With("Secrets", []string{"secret1", "secret2"}).
		WithError("Test error").
		Build()

	// Check all fields are set
	if data["Page"] != 1 {
		t.Error("Page not set correctly in chaining")
	}
	if data["Secrets"] == nil {
		t.Error("Secrets not set correctly in chaining")
	}
	if data["Error"] != true {
		t.Error("Error not set correctly in chaining")
	}
	if data["ErrorMessage"] != "Test error" {
		t.Error("ErrorMessage not set correctly in chaining")
	}
}

func TestTemplateDataBuilder_Build(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	meta := PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"}

	builder := NewTemplateData(r, meta)
	data := builder.Build()

	// Verify it returns a map
	if data == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify it's a proper map[string]any
	if _, ok := data["Title"]; !ok {
		t.Error("Build() did not return expected data structure")
	}
}
