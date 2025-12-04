package database

import (
	"strings"
	"testing"
)

func TestBuildListQuery_BasicSelect(t *testing.T) {
	opts := NewListQueryOptions("users")
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users"`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 0 {
		t.Errorf("Expected 0 args, got %d", len(args))
	}
}

func TestBuildListQuery_WithColumns(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithColumns("id", "name", "email"),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT "id", "name", "email" FROM "users"`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 0 {
		t.Errorf("Expected 0 args, got %d", len(args))
	}
}

func TestBuildListQuery_WithQualifiedColumns(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithColumns("users.id", "users.name", "profiles.bio"),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT "users"."id", "users"."name", "profiles"."bio" FROM "users"`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 0 {
		t.Errorf("Expected 0 args, got %d", len(args))
	}
}

func TestBuildListQuery_CountOnly(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCountOnly(),
		WithCondition(WhereCond("active", Equal, true)),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT COUNT(*) FROM "users" WHERE "active" = $1`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 1 || args[0] != true {
		t.Errorf("Expected args [true], got %v", args)
	}
}

func TestBuildListQuery_WhereEqual(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereCond("status", Equal, "active")),
		WithCondition(WhereCond("age", GreaterThan, 18)),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE "status" = $1 AND "age" > $2`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 2 || args[0] != "active" || args[1] != 18 {
		t.Errorf("Expected args [active, 18], got %v", args)
	}
}

func TestBuildListQuery_WhereLike(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereCond("name", ILike, "%john%")),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE "name" ILIKE $1`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 1 || args[0] != "%john%" {
		t.Errorf("Expected args [%%john%%], got %v", args)
	}
}

func TestBuildListQuery_WhereIn_StringSlice(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereCond("role", In, []string{"admin", "user", "guest"})),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE "role" IN ($1, $2, $3)`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 3 || args[0] != "admin" || args[1] != "user" || args[2] != "guest" {
		t.Errorf("Expected args [admin, user, guest], got %v", args)
	}
}

func TestBuildListQuery_WhereIn_IntSlice(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereCond("age", In, []int{18, 21, 25})),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE "age" IN ($1, $2, $3)`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 3 || args[0] != 18 || args[1] != 21 || args[2] != 25 {
		t.Errorf("Expected args [18, 21, 25], got %v", args)
	}
}

func TestBuildListQuery_WhereAny_StringSlice(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereCond("tags", Any, []string{"vip", "premium"})),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE "tags" = ANY (ARRAY[$1, $2])`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 2 || args[0] != "vip" || args[1] != "premium" {
		t.Errorf("Expected args [vip, premium], got %v", args)
	}
}

func TestBuildListQuery_WhereCustom_SingleParam(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereRawCond("created_at > NOW() - INTERVAL '$1 days'", 7)),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE created_at > NOW() - INTERVAL '$1 days'`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 1 || args[0] != 7 {
		t.Errorf("Expected args [7], got %v", args)
	}
}

func TestBuildListQuery_WhereCustom_MultipleParams(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereRawCond("age BETWEEN $1 AND $2", 18, 65)),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE age BETWEEN $1 AND $2`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 2 || args[0] != 18 || args[1] != 65 {
		t.Errorf("Expected args [18, 65], got %v", args)
	}
}

func TestBuildListQuery_WhereCustom_RepeatedPlaceholder(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereRawCond("(age > $1 OR score > $1)", 100)),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE (age > $1 OR score > $1)`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 1 || args[0] != 100 {
		t.Errorf("Expected args [100], got %v", args)
	}
}

func TestBuildListQuery_WhereCustom_HighNumberedPlaceholder(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithCondition(WhereCond("status", Equal, "active")),
		WithCondition(WhereRawCond("score > $1", 50)),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" WHERE "status" = $1 AND score > $2`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 2 || args[0] != "active" || args[1] != 50 {
		t.Errorf("Expected args [active, 50], got %v", args)
	}
}

func TestBuildListQuery_OrderBy(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithOrderBy("created_at", "DESC"),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" ORDER BY "created_at" DESC`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 0 {
		t.Errorf("Expected 0 args, got %d", len(args))
	}
}

func TestBuildListQuery_OrderBy_QualifiedColumn(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithOrderBy("users.created_at", "ASC"),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" ORDER BY "users"."created_at" ASC`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 0 {
		t.Errorf("Expected 0 args, got %d", len(args))
	}
}

func TestBuildListQuery_LimitOffset(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithLimit(10),
		WithOffset(20),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT * FROM "users" LIMIT $1 OFFSET $2`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 2 || args[0] != 10 || args[1] != 20 {
		t.Errorf("Expected args [10, 20], got %v", args)
	}
}

func TestBuildListQuery_ComplexQuery(t *testing.T) {
	opts := NewListQueryOptions("users",
		WithColumns("id", "name", "email"),
		WithCondition(WhereCond("status", Equal, "active")),
		WithCondition(WhereCond("role", In, []string{"admin", "user"})),
		WithCondition(WhereRawCond("created_at > $1", "2024-01-01")),
		WithOrderBy("created_at", "DESC"),
		WithLimit(50),
		WithOffset(0),
	)
	query, args := BuildListQuery(opts)

	expected := `SELECT "id", "name", "email" FROM "users" WHERE "status" = $1 AND "role" IN ($2, $3) AND created_at > $4 ORDER BY "created_at" DESC LIMIT $5 OFFSET $6`
	if query != expected {
		t.Errorf("Expected query %q, got %q", expected, query)
	}
	if len(args) != 6 {
		t.Errorf("Expected 6 args, got %d: %v", len(args), args)
	}
}

func TestBuildListQuery_SQLInjectionPrevention(t *testing.T) {
	// Attempt SQL injection via table name
	opts := NewListQueryOptions("users; DROP TABLE users;--")
	query, _ := BuildListQuery(opts)

	// Should be properly quoted as a single identifier, making it harmless
	// The entire malicious string becomes a quoted identifier
	expected := `SELECT * FROM "users; DROP TABLE users;--"`
	if query != expected {
		t.Errorf("Expected %q, got %q", expected, query)
	}
	// Verify it doesn't contain unquoted DROP TABLE
	if !strings.Contains(query, `"users; DROP TABLE users;--"`) {
		t.Errorf("Table name not properly quoted: %q", query)
	}
}

func TestJSONText(t *testing.T) {
	result := JSONText("metadata", "name", "user_name")
	expected := `"metadata"->>'name' AS "user_name"`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestJSONText_QualifiedColumn(t *testing.T) {
	result := JSONText("users.metadata", "name", "user_name")
	expected := `"users"."metadata"->>'name' AS "user_name"`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestJSONObject(t *testing.T) {
	result := JSONObject("data", "settings", "user_settings")
	expected := `"data"->'settings' AS "user_settings"`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestJSONPath(t *testing.T) {
	result := JSONPath("data", "user->name", "user_name")
	expected := `"data"->'user'->>'name' AS "user_name"`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
