package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/target/mmk-ui-api/internal/data/database"
	"github.com/redis/go-redis/v9"
)

type deleteSeenDomainRequest struct {
	Ctx     context.Context
	DB      *sql.DB
	Logger  *slog.Logger
	Options clearOptions
}

func deleteSeenDomainRows(req *deleteSeenDomainRequest) (int64, error) {
	if req == nil {
		return 0, errors.New("delete request is required")
	}
	where := make([]string, 0, 3)
	args := make([]any, 0, 3)

	if !req.Options.All {
		where = append(where, fmt.Sprintf("site_id = $%d", len(args)+1))
		args = append(args, req.Options.SiteID)
		if req.Options.Scope != "" {
			where = append(where, fmt.Sprintf("scope = $%d", len(args)+1))
			args = append(args, req.Options.Scope)
		}
		if req.Options.Domain != "" {
			where = append(where, fmt.Sprintf("domain = $%d", len(args)+1))
			args = append(args, req.Options.Domain)
		}
	}

	query := "DELETE FROM seen_domains"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	req.Logger.Info("executing", "query", query, "args", args, "dry_run", req.Options.DryRun)

	if req.Options.DryRun {
		return 0, nil
	}

	res, err := req.DB.ExecContext(req.Ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete seen domains: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("seen domains rows affected: %w", err)
	}
	return rows, nil
}

type purgeSeenRedisRequest struct {
	Ctx     context.Context
	Client  redis.UniversalClient
	Logger  *slog.Logger
	Options clearOptions
}

func purgeSeenRedis(req *purgeSeenRedisRequest) error {
	if req == nil {
		return errors.New("purge request is required")
	}
	patterns := buildSeenPatterns(req.Options)
	if len(patterns) == 0 {
		return nil
	}

	for _, pattern := range patterns {
		req.Logger.Info("scanning redis", "pattern", pattern, "dry_run", req.Options.DryRun)
		iter := req.Client.Scan(req.Ctx, 0, pattern, 1000).Iterator()
		keys := make([]string, 0)
		for iter.Next(req.Ctx) {
			keys = append(keys, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("scan redis: %w", err)
		}
		if len(keys) == 0 {
			continue
		}
		if req.Options.DryRun {
			req.Logger.Info("redis keys matched", "count", len(keys))
			continue
		}
		for start := 0; start < len(keys); start += 100 {
			end := min(start+100, len(keys))
			if err := req.Client.Del(req.Ctx, keys[start:end]...).Err(); err != nil {
				return fmt.Errorf("delete redis keys: %w", err)
			}
		}
		req.Logger.Info("redis keys deleted", "count", len(keys))
	}

	return nil
}

func buildSeenPatterns(opts clearOptions) []string {
	base := "rules:seen:site:"
	switch {
	case opts.All:
		return []string{base + "*"}
	case opts.SiteID == "":
		return nil
	default:
		pattern := base + opts.SiteID + ":scope:"
		if opts.Scope == "" {
			return []string{pattern + "*"}
		}
		pattern += opts.Scope + ":domain:"
		if opts.Domain == "" {
			return []string{pattern + "*"}
		}
		return []string{pattern + opts.Domain}
	}
}

type querySeenDomainRequest struct {
	Ctx     context.Context
	DB      *sql.DB
	Logger  *slog.Logger
	Options *listOptions
}

type seenDomainRow struct {
	SiteID      string
	Scope       string
	Domain      string
	HitCount    int
	FirstSeenAt time.Time
	LastSeenAt  time.Time
}

type querySeenDomainResponse struct {
	Rows  []seenDomainRow
	Total int64
}

func querySeenDomainRows(req *querySeenDomainRequest) (querySeenDomainResponse, error) {
	if req == nil || req.Options == nil {
		return querySeenDomainResponse{}, nil
	}
	conditions := buildListConditions(req.Options)

	countOpts := []database.ListQueryOption{
		database.WithConditions(conditions...),
		database.WithCountOnly(),
	}
	countQuery, countArgs := database.BuildListQuery(
		database.NewListQueryOptions("seen_domains", countOpts...),
	)
	var total int64
	if err := req.DB.QueryRowContext(req.Ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return querySeenDomainResponse{}, fmt.Errorf("count seen domains: %w", err)
	}

	listColumns := []string{"site_id", "scope", "domain", "hit_count", "first_seen_at", "last_seen_at"}
	listOpts := []database.ListQueryOption{
		database.WithColumns(listColumns...),
		database.WithConditions(conditions...),
		database.WithOrderBy("last_seen_at", "DESC"),
	}
	if req.Options.Limit > 0 {
		listOpts = append(listOpts, database.WithLimit(req.Options.Limit))
	}
	if req.Options.Offset > 0 {
		listOpts = append(listOpts, database.WithOffset(req.Options.Offset))
	}
	selectQuery, selectArgs := database.BuildListQuery(
		database.NewListQueryOptions("seen_domains", listOpts...),
	)

	req.Logger.Debug("querying seen domains", "query", selectQuery, "args", selectArgs)

	rows, err := req.DB.QueryContext(req.Ctx, selectQuery, selectArgs...)
	if err != nil {
		return querySeenDomainResponse{}, fmt.Errorf("list seen domains: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && req.Logger != nil {
			req.Logger.Warn("close seen domain rows failed", "error", closeErr)
		}
	}()

	out := make([]seenDomainRow, 0)
	for rows.Next() {
		var row seenDomainRow
		if scanErr := rows.Scan(&row.SiteID, &row.Scope, &row.Domain, &row.HitCount, &row.FirstSeenAt, &row.LastSeenAt); scanErr != nil {
			return querySeenDomainResponse{}, fmt.Errorf("scan seen domain row: %w", scanErr)
		}
		out = append(out, row)
	}
	if iterErr := rows.Err(); iterErr != nil {
		return querySeenDomainResponse{}, fmt.Errorf("list seen domains rows: %w", iterErr)
	}

	return querySeenDomainResponse{Rows: out, Total: total}, nil
}

func buildListConditions(opts *listOptions) []database.Condition {
	if opts == nil {
		return nil
	}
	conditions := make([]database.Condition, 0, 3)
	if opts.SiteID != "" {
		conditions = append(conditions, database.WhereCond("site_id", database.Equal, opts.SiteID))
	}
	if opts.Scope != "" {
		conditions = append(conditions, database.WhereCond("scope", database.Equal, opts.Scope))
	}
	if opts.Domain != "" {
		conditions = append(conditions, database.WhereCond("domain", database.ILike, "%"+opts.Domain+"%"))
	}
	return conditions
}

type inspectSeenRedisRequest struct {
	Ctx     context.Context
	Client  redis.UniversalClient
	Logger  *slog.Logger
	Options *listOptions
}

type redisEntry struct {
	Key    string
	SiteID string
	Scope  string
	Domain string
	TTL    time.Duration
}

type inspectSeenRedisResponse struct {
	Entries []redisEntry
	Total   int
}

func inspectSeenRedis(req *inspectSeenRedisRequest) (inspectSeenRedisResponse, error) {
	if req == nil || req.Options == nil {
		return inspectSeenRedisResponse{}, nil
	}
	patterns := buildSeenQueryPatterns(req.Options)
	if len(patterns) == 0 {
		return inspectSeenRedisResponse{}, nil
	}

	collector := redisCollector{limit: req.Options.Limit}
	for _, pattern := range patterns {
		req.Logger.Info("scanning redis", "pattern", pattern)
		if err := collector.scanPattern(req, pattern); err != nil {
			return inspectSeenRedisResponse{}, err
		}
	}
	return collector.result(), nil
}

type redisCollector struct {
	entries []redisEntry
	total   int
	limit   int
}

func (c *redisCollector) scanPattern(req *inspectSeenRedisRequest, pattern string) error {
	if req == nil {
		return errors.New("inspect redis request is required")
	}
	iter := req.Client.Scan(req.Ctx, 0, pattern, 1000).Iterator()
	for iter.Next(req.Ctx) {
		if err := c.addKey(req, iter.Val()); err != nil {
			return err
		}
	}
	return iter.Err()
}

func (c *redisCollector) addKey(req *inspectSeenRedisRequest, key string) error {
	if req == nil {
		return nil
	}
	c.total++
	if c.limit > 0 && len(c.entries) >= c.limit {
		return nil
	}

	siteID, scope, domain, err := parseSeenRedisKey(key)
	if err != nil {
		req.Logger.Warn("skipping redis key", "key", key, "error", err)
		return nil
	}

	ttl, err := req.Client.TTL(req.Ctx, key).Result()
	if err != nil {
		return fmt.Errorf("query redis ttl for key %q: %w", key, err)
	}

	c.entries = append(c.entries, redisEntry{
		Key:    key,
		SiteID: siteID,
		Scope:  scope,
		Domain: domain,
		TTL:    ttl,
	})
	return nil
}

func (c *redisCollector) result() inspectSeenRedisResponse {
	sort.Slice(c.entries, func(i, j int) bool {
		if c.entries[i].SiteID == c.entries[j].SiteID {
			if c.entries[i].Scope == c.entries[j].Scope {
				return c.entries[i].Domain < c.entries[j].Domain
			}
			return c.entries[i].Scope < c.entries[j].Scope
		}
		return c.entries[i].SiteID < c.entries[j].SiteID
	})

	return inspectSeenRedisResponse{
		Entries: c.entries,
		Total:   c.total,
	}
}

func buildSeenQueryPatterns(opts *listOptions) []string {
	if opts == nil || (!opts.All && opts.SiteID == "") {
		return nil
	}

	base := "rules:seen:site:"
	sitePart := "*"
	if opts.SiteID != "" {
		sitePart = opts.SiteID
	}
	scopePart := "*"
	if opts.Scope != "" {
		scopePart = opts.Scope
	}
	domainPart := "*"
	if opts.Domain != "" {
		domainPart = "*" + opts.Domain + "*"
	}

	return []string{base + sitePart + ":scope:" + scopePart + ":domain:" + domainPart}
}

var errUnexpectedSeenRedisKeyFormat = errors.New("unexpected seen redis key format")

func parseSeenRedisKey(key string) (string, string, string, error) {
	parts := strings.Split(key, ":")
	if len(parts) < 8 {
		return "", "", "", errUnexpectedSeenRedisKeyFormat
	}
	if parts[0] != "rules" || parts[1] != "seen" || parts[2] != "site" || parts[4] != "scope" || parts[6] != "domain" {
		return "", "", "", errUnexpectedSeenRedisKeyFormat
	}
	siteID := parts[3]
	scope := parts[5]
	domain := strings.Join(parts[7:], ":")
	return siteID, scope, domain, nil
}

func printSeenDomainRows(resp querySeenDomainResponse, opts *listOptions) error {
	if opts == nil {
		return errors.New("list options are required")
	}
	displayLimit := max(opts.Limit, 0)
	if err := writef(os.Stdout, "\nPostgres seen_domains results"); err != nil {
		return fmt.Errorf("write seen domains header: %w", err)
	}
	if err := writeSeenDomainHeaderInfo(displayLimit, opts.Offset); err != nil {
		return err
	}
	if err := writeln(os.Stdout); err != nil {
		return fmt.Errorf("write seen domains header newline: %w", err)
	}

	if len(resp.Rows) == 0 {
		return printSeenDomainsEmpty()
	}

	if err := renderSeenDomainTable(resp.Rows); err != nil {
		return err
	}

	if err := writef(os.Stdout, "Total matching rows: %d\n", resp.Total); err != nil {
		return fmt.Errorf("write seen domains total: %w", err)
	}
	if opts.Limit > 0 && len(resp.Rows) == opts.Limit && int64(opts.Offset+opts.Limit) < resp.Total {
		if err := writeln(os.Stdout, "More rows available; adjust --offset or --limit to view additional data."); err != nil {
			return fmt.Errorf("write seen domains more-rows message: %w", err)
		}
	}
	return nil
}

func writeSeenDomainHeaderInfo(limit, offset int) error {
	switch {
	case limit > 0:
		if err := writef(os.Stdout, " (limit %d, offset %d)", limit, offset); err != nil {
			return fmt.Errorf("write seen domains limit: %w", err)
		}
	case offset > 0:
		if err := writef(os.Stdout, " (offset %d)", offset); err != nil {
			return fmt.Errorf("write seen domains offset: %w", err)
		}
	}
	return nil
}

func printSeenDomainsEmpty() error {
	if err := writeln(os.Stdout, "  (no rows found)"); err != nil {
		return fmt.Errorf("write seen domains empty message: %w", err)
	}
	return nil
}

func renderSeenDomainTable(rows []seenDomainRow) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if err := writeln(tw, "SITE ID\tSCOPE\tDOMAIN\tHITS\tFIRST SEEN (UTC)\tLAST SEEN (UTC)"); err != nil {
		return fmt.Errorf("write seen domains header row: %w", err)
	}

	for _, row := range rows {
		if err := writef(
			tw,
			"%s\t%s\t%s\t%d\t%s\t%s\n",
			row.SiteID,
			row.Scope,
			row.Domain,
			row.HitCount,
			formatTimestamp(row.FirstSeenAt),
			formatTimestamp(row.LastSeenAt),
		); err != nil {
			return fmt.Errorf("write seen domains row: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush seen domains table: %w", err)
	}
	return nil
}

func printRedisEntries(resp inspectSeenRedisResponse, opts *listOptions) error {
	if opts == nil {
		return errors.New("list options are required")
	}
	displayLimit := max(opts.Limit, 0)
	if err := writef(os.Stdout, "\nRedis seen cache entries"); err != nil {
		return fmt.Errorf("write redis entries header: %w", err)
	}
	if err := writeRedisEntriesHeaderInfo(displayLimit); err != nil {
		return err
	}
	if err := writeln(os.Stdout); err != nil {
		return fmt.Errorf("write redis entries header newline: %w", err)
	}

	if len(resp.Entries) == 0 {
		return printRedisEntriesEmpty()
	}

	if err := renderRedisEntriesTable(resp.Entries); err != nil {
		return err
	}

	if err := writef(os.Stdout, "Total keys matched: %d\n", resp.Total); err != nil {
		return fmt.Errorf("write redis entries total: %w", err)
	}
	if opts.Limit > 0 && resp.Total > len(resp.Entries) {
		if err := writeln(os.Stdout, "More keys available; increase --limit to view additional entries."); err != nil {
			return fmt.Errorf("write redis entries more-keys message: %w", err)
		}
	}
	return nil
}

func writeRedisEntriesHeaderInfo(limit int) error {
	if limit == 0 {
		return nil
	}

	if err := writef(os.Stdout, " (showing up to %d)", limit); err != nil {
		return fmt.Errorf("write redis entries limit: %w", err)
	}
	return nil
}

func printRedisEntriesEmpty() error {
	if err := writeln(os.Stdout, "  (no keys matched)"); err != nil {
		return fmt.Errorf("write redis entries empty message: %w", err)
	}
	return nil
}

func renderRedisEntriesTable(entries []redisEntry) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if err := writeln(tw, "SITE ID\tSCOPE\tDOMAIN\tTTL\tKEY"); err != nil {
		return fmt.Errorf("write redis entries header row: %w", err)
	}

	for _, entry := range entries {
		if err := writef(
			tw,
			"%s\t%s\t%s\t%s\t%s\n",
			entry.SiteID,
			entry.Scope,
			entry.Domain,
			formatRedisTTL(entry.TTL),
			entry.Key,
		); err != nil {
			return fmt.Errorf("write redis entry row: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush redis entries table: %w", err)
	}
	return nil
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatRedisTTL(ttl time.Duration) string {
	if ttl == -1 {
		return "no expiry"
	}
	if ttl == -2 {
		return "missing"
	}
	if ttl < 0 {
		return ttl.String()
	}
	return ttl.Round(time.Millisecond).String()
}
