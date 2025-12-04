package database

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ConditionType string

const (
	Equal              ConditionType = "="
	NotEqual           ConditionType = "!="
	GreaterThan        ConditionType = ">"
	LessThan           ConditionType = "<"
	LessThanOrEqual    ConditionType = "<="
	GreaterThanOrEqual ConditionType = ">="
	Like               ConditionType = "LIKE"
	ILike              ConditionType = "ILIKE"
	In                 ConditionType = "IN"
	Any                ConditionType = "ANY"
	Custom             ConditionType = "CUSTOM"
	defaultLimit                     = -1
	defaultOffset                    = -1
	// maxAliasParts is the maximum number of parts when splitting on " AS ".
	maxAliasParts = 2
	// expectedJSONRegexMatches is the expected number of matches from JSON regex.
	expectedJSONRegexMatches = 3
)

type Condition struct {
	Field    string
	Type     ConditionType
	Value    any
	rawQuery *string
}

func WhereCond(field string, condType ConditionType, value any) Condition {
	if condType == Custom {
		//nolint:forbidigo // panic prevents misuse; custom conditions must provide raw SQL via WhereRawCond.
		panic("Use WhereRawCond for Custom type")
	}
	return Condition{
		rawQuery: nil,
		Field:    field,
		Type:     condType,
		Value:    value,
	}
}

func WhereRawCond(rawQuery string, params ...any) Condition {
	queryStr := rawQuery
	var value any = params
	if len(params) == 0 {
		value = nil
	} else if len(params) == 1 {
		value = params[0]
	}
	// For multiple parameters, keep the slice as-is so handleCustomCondition can process it

	return Condition{
		Field:    "",
		Type:     Custom,
		rawQuery: &queryStr,
		Value:    value,
	}
}

type ListQueryOptions struct {
	Table      string
	Columns    []string
	CountOnly  bool
	Conditions []Condition
	OrderBy    string
	OrderDir   string
	Limit      int
	Offset     int
}

type ListQueryOption func(*ListQueryOptions)

func NewListQueryOptions(table string, opts ...ListQueryOption) *ListQueryOptions {
	options := &ListQueryOptions{
		Table:      table,
		Columns:    []string{},
		CountOnly:  false,
		Conditions: []Condition{},
		OrderBy:    "",
		OrderDir:   "",
		Limit:      defaultLimit,
		Offset:     defaultOffset,
	}

	for _, opt := range opts {
		opt(options)
	}
	return options
}

// WithColumns sets the columns to select.
func WithColumns(cols ...string) ListQueryOption {
	return func(o *ListQueryOptions) {
		o.Columns = cols
	}
}

// WithCondition adds a single condition.
func WithCondition(cond Condition) ListQueryOption {
	return func(o *ListQueryOptions) {
		o.Conditions = append(o.Conditions, cond)
	}
}

// WithConditions sets the entire list of conditions.
func WithConditions(conds ...Condition) ListQueryOption {
	return func(o *ListQueryOptions) {
		o.Conditions = conds
	}
}

// WithOrderBy sets the ordering column and direction.
func WithOrderBy(column, direction string) ListQueryOption {
	return func(o *ListQueryOptions) {
		o.OrderBy = column
		o.OrderDir = direction
	}
}

// WithLimit sets the limit. Accepts 0.
func WithLimit(limit int) ListQueryOption {
	return func(o *ListQueryOptions) {
		if limit >= 0 {
			o.Limit = limit
		}
	}
}

// WithOffset sets the offset. Accepts 0.
func WithOffset(offset int) ListQueryOption {
	return func(o *ListQueryOptions) {
		if offset >= 0 {
			o.Offset = offset
		}
	}
}

// WithCountOnly sets the query to count only.
func WithCountOnly() ListQueryOption {
	return func(o *ListQueryOptions) {
		o.CountOnly = true
	}
}

// sanitizeIdentifier wraps a single string identifier for sanitization.
func sanitizeIdentifier(ident string) string {
	return pgx.Identifier{ident}.Sanitize()
}

// sanitizeQualifiedIdentifier sanitizes qualified identifiers like "table.column" or "schema.table.column".
// It splits on '.' and uses pgx.Identifier to properly quote each part.
func sanitizeQualifiedIdentifier(ident string) string {
	parts := strings.Split(ident, ".")
	return pgx.Identifier(parts).Sanitize()
}

// JSONText creates a JSON text extraction column specification (using ->>).
// The column parameter is sanitized to support qualified identifiers like "table.column".
func JSONText(column, path, alias string) string {
	return fmt.Sprintf("%s->>'%s' AS %s",
		sanitizeQualifiedIdentifier(column),
		sanitizeJSONPath(path),
		sanitizeIdentifier(alias))
}

// JSONObject creates a JSON object extraction column specification (using ->).
// The column parameter is sanitized to support qualified identifiers like "table.column".
func JSONObject(column, path, alias string) string {
	return fmt.Sprintf("%s->'%s' AS %s",
		sanitizeQualifiedIdentifier(column),
		sanitizeJSONPath(path),
		sanitizeIdentifier(alias))
}

// JSONPath creates a nested JSON path extraction column specification.
// The column parameter is sanitized to support qualified identifiers like "table.column".
func JSONPath(column, path, alias string) string {
	parts := strings.Split(path, "->")
	if len(parts) == 1 {
		// Single level, use ->> for text extraction
		return JSONText(column, path, alias)
	}

	// Multi-level path: all but last use ->, last uses ->>
	var pathBuilder strings.Builder
	pathBuilder.WriteString(sanitizeQualifiedIdentifier(column))

	for i, part := range parts {
		if i == len(parts)-1 {
			pathBuilder.WriteString(fmt.Sprintf("->>'%s'", sanitizeJSONPath(part)))
		} else {
			// Intermediate parts: use -> for object navigation
			pathBuilder.WriteString(fmt.Sprintf("->'%s'", sanitizeJSONPath(part)))
		}
	}

	pathBuilder.WriteString(" AS ")
	pathBuilder.WriteString(sanitizeIdentifier(alias))
	return pathBuilder.String()
}

// sanitizeJSONPath sanitizes JSON path components to prevent injection.
// It allows alphanumeric characters, underscores, and hyphens.
func sanitizeJSONPath(path string) string {
	// Remove any characters that aren't alphanumeric, underscore, or hyphen
	// This prevents JSON injection while allowing common field names
	var result strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		}
		// Skip invalid characters (don't add them to result)
	}

	return result.String()
}

// processColumnSpec processes a column specification, handling aliases and JSON expressions.
// Supports formats like:
// - "column" -> "column"
// - "column AS alias" -> "column" AS "alias"
// - "json_col->>'path' AS alias" -> "json_col"->>'path' AS "alias".
func processColumnSpec(columnSpec string) string {
	// Check if it contains " AS " (case insensitive)
	asRegex := regexp.MustCompile(`(?i)\s+AS\s+`)
	if asRegex.MatchString(columnSpec) {
		// Split on " AS " to get column expression and alias
		parts := asRegex.Split(columnSpec, maxAliasParts)
		if len(parts) == maxAliasParts {
			columnExpr := strings.TrimSpace(parts[0])
			alias := strings.TrimSpace(parts[1])

			// Process the column expression part
			processedExpr := processColumnExpression(columnExpr)

			// Sanitize the alias
			sanitizedAlias := sanitizeIdentifier(alias)

			return fmt.Sprintf("%s AS %s", processedExpr, sanitizedAlias)
		}
	}

	// No alias, process as simple column expression
	return processColumnExpression(columnSpec)
}

// processColumnExpression processes a column expression, handling JSON operators and qualified identifiers.
func processColumnExpression(expr string) string {
	// Check if it contains JSON operators (-> or ->>)
	if strings.Contains(expr, "->") {
		// This is a JSON expression, handle it carefully
		return processJSONExpression(expr)
	}

	// Check if it's a qualified identifier (contains '.')
	if strings.Contains(expr, ".") {
		return sanitizeQualifiedIdentifier(expr)
	}

	// Simple column name, sanitize it
	return sanitizeIdentifier(expr)
}

// processJSONExpression processes JSON path expressions like "column->>'path'" or "table.column->'path'".
func processJSONExpression(expr string) string {
	// Pattern to match JSON expressions with optional qualified identifiers
	// This handles: column->>'path', table.column->'path', schema.table.column->'path'->>'subpath', etc.
	jsonRegex := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.]*)((?:->>'[^']*'|(?:->'[^']*'))+)$`)

	matches := jsonRegex.FindStringSubmatch(expr)
	if len(matches) == expectedJSONRegexMatches {
		columnName := matches[1]
		jsonPath := matches[2]

		// Sanitize the column name (may be qualified like "table.column")
		var sanitizedColumn string
		if strings.Contains(columnName, ".") {
			sanitizedColumn = sanitizeQualifiedIdentifier(columnName)
		} else {
			sanitizedColumn = sanitizeIdentifier(columnName)
		}
		validatedPath := validateJSONPath(jsonPath)

		return sanitizedColumn + validatedPath
	}

	// If it doesn't match our JSON pattern, return empty string to skip safely
	// This prevents invalid SQL from being generated
	return ""
}

// validateJSONPath validates and sanitizes JSON path expressions.
func validateJSONPath(path string) string {
	// Pattern to match valid JSON path components
	pathRegex := regexp.MustCompile(`(->>'[a-zA-Z0-9_-]*'|(?:->'[a-zA-Z0-9_-]*'))`)

	// Find all valid path components
	matches := pathRegex.FindAllString(path, -1)

	// Reconstruct the path from valid components
	return strings.Join(matches, "")
}

// buildSelectClause generates the SELECT part of the query with sanitized columns.
func buildSelectClause(options *ListQueryOptions) string {
	if options == nil {
		return ""
	}
	if options.CountOnly {
		return "SELECT COUNT(*) "
	}
	if len(options.Columns) == 0 {
		return "SELECT * "
	}

	// Process columns to handle aliases and JSON expressions
	processedColumns := make([]string, len(options.Columns))
	for i, col := range options.Columns {
		processedColumns[i] = processColumnSpec(col)
	}

	return fmt.Sprintf("SELECT %s ", strings.Join(processedColumns, ", "))
}

// buildPaginationAndOrderClause generates ORDER BY, LIMIT, OFFSET parts with sanitized OrderBy and validated OrderDir.
func buildPaginationAndOrderClause(
	options *ListQueryOptions,
	startParamIndex int,
	initialArgs []any,
) (string, []any) {
	if options == nil {
		return "", initialArgs
	}

	var clause strings.Builder
	args := initialArgs
	paramCount := startParamIndex

	if options.OrderBy != "" {
		clause.WriteString(" ORDER BY ")
		clause.WriteString(sanitizeQualifiedIdentifier(options.OrderBy))
		upperOrderDir := strings.ToUpper(options.OrderDir)
		if upperOrderDir == "ASC" || upperOrderDir == "DESC" {
			clause.WriteString(" ")
			clause.WriteString(upperOrderDir)
		}
	}

	// Add LIMIT clause only if it was explicitly set (not the default sentinel)
	if options.Limit != defaultLimit {
		clause.WriteString(fmt.Sprintf(" LIMIT $%d", paramCount))
		args = append(args, options.Limit)
		paramCount++
	}

	// Add OFFSET clause only if it was explicitly set (not the default sentinel)
	if options.Offset != defaultOffset {
		clause.WriteString(fmt.Sprintf(" OFFSET $%d", paramCount))
		args = append(args, options.Offset)
	}

	return clause.String(), args
}

// BuildListQuery constructs a SQL query string and arguments from options, sanitizing identifiers.
// It handles SELECT, WHERE, ORDER BY, LIMIT, and OFFSET clauses.
//
// Example usage:
//
//	options := NewListQueryOptions("users",
//		WithColumns("id", "name", "table.column"),
//		WithCondition(WhereCond("age", GreaterThan, 18)),
//		WithCondition(WhereCond("status", Equal, "active")),
//		WithCondition(WhereCond("tags", In, []string{"admin", "user"})),
//		WithCondition(WhereRawCond("custom_field = $1", "value")),
//		WithOrderBy("created_at", "DESC"),
//		WithLimit(10),
//		WithOffset(0),
//	)
//
//	query, args := BuildListQuery(options)
//	fmt.Println(query)
//	fmt.Println(args)
func BuildListQuery(options *ListQueryOptions) (string, []any) {
	if options == nil {
		return "", nil
	}

	var query strings.Builder

	// SELECT ... FROM ...
	query.WriteString(buildSelectClause(options))
	query.WriteString("FROM ")
	query.WriteString(sanitizeIdentifier(options.Table)) // Sanitize table name

	// WHERE ...
	whereClause, whereArgs, nextParamCount := buildWhereClause(options.Conditions, 1)
	if whereClause != "" {
		query.WriteString(" ")
		query.WriteString(whereClause)
	}

	// return early for CountOnly
	if options.CountOnly {
		return query.String(), whereArgs
	}

	// ORDER BY ... LIMIT ... OFFSET ...
	paginationOrderClause, finalArgs := buildPaginationAndOrderClause(
		options,
		nextParamCount,
		whereArgs,
	)
	if paginationOrderClause != "" {
		query.WriteString(paginationOrderClause)
	}

	return query.String(), finalArgs
}

func handleStandardCondition(
	cond Condition,
	sanitizedField string,
	paramCount int,
) (string, []any, int) {
	if sanitizedField == "" {
		return "", []any{}, paramCount
	}
	conditionStr := fmt.Sprintf("%s %s $%d", sanitizedField, cond.Type, paramCount)
	args := []any{cond.Value}
	return conditionStr, args, paramCount + 1
}

func handleInCondition(cond Condition, sanitizedField string, paramCount int) (string, []any, int) {
	if sanitizedField == "" {
		return "", []any{}, paramCount
	}

	// Accept any slice type via reflection
	rv := reflect.ValueOf(cond.Value)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return "", []any{}, paramCount
	}

	placeholders := make([]string, rv.Len())
	args := make([]any, rv.Len())
	currentParam := paramCount
	for i := range rv.Len() {
		placeholders[i] = fmt.Sprintf("$%d", currentParam)
		args[i] = rv.Index(i).Interface()
		currentParam++
	}
	conditionStr := fmt.Sprintf("%s IN (%s)", sanitizedField, strings.Join(placeholders, ", "))
	return conditionStr, args, currentParam
}

func handleAnyCondition(
	cond Condition,
	sanitizedField string,
	paramCount int,
) (string, []any, int) {
	if sanitizedField == "" {
		return "", []any{}, paramCount
	}

	// Accept any slice type via reflection
	rv := reflect.ValueOf(cond.Value)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return "", []any{}, paramCount
	}

	placeholders := make([]string, rv.Len())
	args := make([]any, rv.Len())
	currentParam := paramCount
	for i := range rv.Len() {
		placeholders[i] = fmt.Sprintf("$%d", currentParam)
		args[i] = rv.Index(i).Interface()
		currentParam++
	}
	conditionStr := fmt.Sprintf(
		"%s = ANY (ARRAY[%s])",
		sanitizedField,
		strings.Join(placeholders, ", "),
	)
	return conditionStr, args, currentParam
}

func handleCustomCondition(cond Condition, paramCount int) (string, []any, int) {
	args := []any{}
	if cond.rawQuery == nil || *cond.rawQuery == "" {
		// Handle error or return empty if RawQuery is required for Custom type
		return "", []any{}, paramCount
	}
	conditionStr := *cond.rawQuery

	if cond.Value == nil {
		return conditionStr, args, paramCount
	}

	// NOTE: RawQuery itself is NOT sanitized here.
	// Normalize to slice: treat any []any as-is, otherwise wrap single value
	var params []any
	if paramSlice, ok := cond.Value.([]any); ok {
		params = paramSlice
	} else {
		params = []any{cond.Value}
	}

	// Use regex to replace placeholders, handling $10 vs $1 correctly
	// Build a map of original placeholder index to new parameter number
	currentParam := paramCount
	re := regexp.MustCompile(`\$(\d+)`)
	idxMap := make(map[int]int)
	conditionStr = re.ReplaceAllStringFunc(conditionStr, func(m string) string {
		n, err := strconv.Atoi(m[1:])
		if err != nil {
			return m
		}
		if _, ok := idxMap[n]; !ok {
			// Guard bounds: ensure n-1 is within params slice
			if n < 1 || n > len(params) {
				// Invalid placeholder index, skip replacement
				return m
			}
			idxMap[n] = currentParam
			args = append(args, params[n-1])
			currentParam++
		}
		return fmt.Sprintf("$%d", idxMap[n])
	})

	return conditionStr, args, currentParam
}

// processCondition processes a single condition and returns the SQL string, args, and next param count.
func processCondition(cond Condition, paramCount int) (string, []any, int) {
	// Sanitize field name unless it's a Custom query or field is intentionally empty
	sanitizedField := ""
	if cond.Type != Custom && cond.Field != "" {
		sanitizedField = sanitizeIdentifier(cond.Field)
	}

	switch cond.Type {
	case Custom:
		return handleCustomCondition(cond, paramCount)
	case In:
		if sanitizedField == "" {
			return "", []any{}, paramCount
		}
		return handleInCondition(cond, sanitizedField, paramCount)
	case Any:
		if sanitizedField == "" {
			return "", []any{}, paramCount
		}
		return handleAnyCondition(cond, sanitizedField, paramCount)
	case Equal, NotEqual, GreaterThan, LessThan, LessThanOrEqual, GreaterThanOrEqual, Like, ILike:
		if sanitizedField == "" {
			return "", []any{}, paramCount
		}
		return handleStandardCondition(cond, sanitizedField, paramCount)
	}
	return "", []any{}, paramCount
}

// buildWhereClause generates the WHERE part of the query with sanitized fields and manages parameters.
func buildWhereClause(inputConditions []Condition, startParamIndex int) (string, []any, int) {
	conditions := make([]string, 0, len(inputConditions))
	args := []any{}
	paramCount := startParamIndex

	for _, cond := range inputConditions {
		conditionStr, newArgs, nextParamCount := processCondition(cond, paramCount)
		if conditionStr != "" {
			conditions = append(conditions, conditionStr)
			args = append(args, newArgs...)
			paramCount = nextParamCount
		}
	}

	if len(conditions) == 0 {
		return "", args, paramCount
	}
	return "WHERE " + strings.Join(conditions, " AND "), args, paramCount
}
