package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/http/uiutil"
)

// Deps holds optional dependencies for constructing the core template func map.
type Deps struct {
	Template           **template.Template
	ContentTemplateFor func(string) string
}

// Funcs returns a template.FuncMap containing helpers that are broadly useful across templates.
func Funcs(deps Deps) template.FuncMap {
	funcs := template.FuncMap{
		"sectionTmpl":   deps.ContentTemplateFor,
		"friendlyTime":  createFriendlyTimeFunc(),
		"timeTag":       createTimeTagFunc(),
		"slice":         func(nums ...int) []int { return nums },
		"add":           func(a, b int) int { return a + b },
		"sub":           func(a, b int) int { return a - b },
		"contains":      strings.Contains,
		"formatNumber":  formatNumberTemplate,
		"severityClass": severityClass,
		"truncateText":  TruncateText,
		"strLen":        StrLen,
	}

	addRenderFuncs(funcs, deps)
	return funcs
}

func addRenderFuncs(funcs template.FuncMap, deps Deps) {
	funcs["renderSection"] = func(page string, data any) (template.HTML, error) {
		if deps.Template == nil || *deps.Template == nil {
			return "", errors.New("template not initialized")
		}
		var buf bytes.Buffer
		if err := (*deps.Template).ExecuteTemplate(&buf, deps.ContentTemplateFor(page), data); err != nil {
			return "", err
		}
		// #nosec G203 - The HTML here is rendered by our own trusted templates (html/template),
		// and is embedded back into the same template set. User-provided values were already
		// auto-escaped during ExecuteTemplate above.
		return template.HTML(buf.String()), nil
	}

	funcs["toJSON"] = func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func createFriendlyTimeFunc() func(any) string {
	return func(ts any) string {
		var t0 time.Time
		switch v := ts.(type) {
		case time.Time:
			t0 = v
		case *time.Time:
			if v != nil {
				t0 = *v
			}
		default:
			return ""
		}
		if t0.IsZero() {
			return ""
		}
		return uiutil.FormatFriendlyDateTime(t0)
	}
}

func createTimeTagFunc() func(any) template.HTML {
	return func(ts any) template.HTML {
		var t0 time.Time
		switch v := ts.(type) {
		case time.Time:
			t0 = v
		case *time.Time:
			if v != nil {
				t0 = *v
			}
		default:
			return ""
		}
		if t0.IsZero() {
			return ""
		}
		friendly := t0.Local().Format("Jan 2, 2006 3:04:05 PM")
		dt := t0.UTC().Format(time.RFC3339)
		title := t0.Local().Format(time.RFC1123)
		// #nosec G203 - The HTML here is constructed from trusted, escaped values only
		return template.HTML(
			fmt.Sprintf(
				"<time datetime=\"%s\" title=\"%s\">%s</time>",
				dt,
				template.HTMLEscapeString(title),
				template.HTMLEscapeString(friendly),
			),
		)
	}
}

// formatNumberTemplate formats any integer type with comma separators for thousands.
// Handles negative numbers and values of any size.
// Accepts int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64.
func formatNumberTemplate(v any) string {
	// Convert to int64 for signed types, uint64 for unsigned
	var s string
	var neg bool

	switch x := v.(type) {
	case int:
		s, neg = formatInt64(int64(x))
	case int64:
		s, neg = formatInt64(x)
	case int32:
		s, neg = formatInt64(int64(x))
	case int16:
		s, neg = formatInt64(int64(x))
	case int8:
		s, neg = formatInt64(int64(x))
	case uint, uint64, uint32, uint16, uint8:
		s = formatUint64(x)
	default:
		return fmt.Sprint(v)
	}

	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}

	return formatWithCommas(s, neg)
}

// formatInt64 converts int64 to string and tracks sign.
func formatInt64(x int64) (string, bool) {
	if x < 0 {
		return strconv.FormatUint(uint64(-x), 10), true
	}
	return strconv.FormatUint(uint64(x), 10), false
}

// formatUint64 converts any unsigned integer to string.
func formatUint64(v any) string {
	switch x := v.(type) {
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	default:
		return "0"
	}
}

// formatWithCommas formats a numeric string with comma separators.
func formatWithCommas(s string, neg bool) string {
	var b strings.Builder
	b.Grow(len(s) + (len(s)-1)/3)

	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}

	b.WriteString(s[:prefix])
	for i := prefix; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}

	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func severityClass(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "badge-danger"
	case "high":
		return "badge-warning"
	case "medium":
		return "badge-info"
	case "low":
		return "badge-secondary"
	default:
		return "badge-light"
	}
}

// TruncateText truncates a string to a maximum number of runes (not bytes).
// Adds an ellipsis (…) when truncated for visual clarity.
// The maxLen parameter can be any numeric type for template flexibility.
func TruncateText(s string, maxLen any) string {
	n, ok := toIntSafe(maxLen)
	if !ok || n <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= n {
		return s
	}

	// Add ellipsis if we have room (need at least 1 char + ellipsis)
	if n > 1 {
		return string(runes[:n-1]) + "…"
	}

	return string(runes[:1])
}

func toIntSafe(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

// StrLen returns the length of a string; exposed for template testing parity.
func StrLen(s string) int {
	return len(s)
}
