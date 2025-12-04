package bootstrap

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/redis/go-redis/v9"
)

// DatabaseConfig contains configuration for database connections.
type DatabaseConfig struct {
	DBConfig    config.DBConfig
	RedisConfig config.RedisConfig
	Logger      *slog.Logger
}

// ConnectDB establishes a connection to the PostgreSQL database.
func ConnectDB(cfg DatabaseConfig) (*sql.DB, error) {
	// Build DSN using url.URL to safely handle special characters in credentials
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.DBConfig.User, cfg.DBConfig.Password),
		Host:   net.JoinHostPort(cfg.DBConfig.Host, strconv.Itoa(cfg.DBConfig.Port)),
		Path:   "/" + cfg.DBConfig.Name,
	}
	q := u.Query()
	q.Set("sslmode", cfg.DBConfig.SSLMode)
	u.RawQuery = q.Encode()
	dsn := u.String()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if pingErr := db.PingContext(ctx); pingErr != nil {
		if closeErr := db.Close(); closeErr != nil {
			pingErr = errors.Join(pingErr, fmt.Errorf("close database connection: %w", closeErr))
		}
		return nil, fmt.Errorf("ping database: %w", pingErr)
	}

	if cfg.Logger != nil {
		cfg.Logger.Info("database connected",
			"host", cfg.DBConfig.Host,
			"port", cfg.DBConfig.Port,
			"database", cfg.DBConfig.Name,
		)
	}

	return db, nil
}

// ConnectRedis establishes a connection to Redis.
//
//nolint:ireturn // returning redis.UniversalClient lets us pick single, sentinel, or cluster clients at runtime.
func ConnectRedis(cfg DatabaseConfig) (redis.UniversalClient, error) {
	var (
		client   redis.UniversalClient
		addrDesc string
		err      error
	)

	switch {
	case cfg.RedisConfig.UseCluster:
		client, addrDesc, err = newClusterClient(cfg.RedisConfig)
	case cfg.RedisConfig.UseSentinel:
		client, addrDesc, err = newSentinelClient(cfg.RedisConfig)
	default:
		client, addrDesc, err = newDirectClient(cfg.RedisConfig)
	}
	if err != nil {
		return nil, err
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if pingErr := client.Ping(ctx).Err(); pingErr != nil {
		if closeErr := client.Close(); closeErr != nil {
			pingErr = errors.Join(pingErr, fmt.Errorf("close redis client: %w", closeErr))
		}
		return nil, fmt.Errorf("ping redis: %w", pingErr)
	}

	if cfg.Logger != nil {
		// Log connection without credentials
		if u, parseErr := url.Parse(addrDesc); parseErr == nil && u.User != nil {
			u.User = url.User("*")
			addrDesc = u.Redacted()
		} else if i := strings.LastIndex(addrDesc, "@"); i > -1 {
			addrDesc = addrDesc[i+1:]
		}

		cfg.Logger.Info("redis connected", "addr", addrDesc)
	}

	return client, nil
}

//nolint:ireturn // returning redis.UniversalClient keeps client selection flexible.
func newClusterClient(cfg config.RedisConfig) (redis.UniversalClient, string, error) {
	addrs := normalizeAddrs(cfg.ClusterNodes)
	password := cfg.Password
	username := ""
	var tlsConfig *tls.Config

	if len(addrs) == 0 {
		addr, parsedUsername, parsedPassword, parsedTLS, err := clusterFallbackFromURI(cfg.URI, password)
		if err != nil {
			return nil, "", err
		}

		if addr != "" {
			addrs = []string{addr}
			username = parsedUsername
			password = parsedPassword
			tlsConfig = parsedTLS
		}
	}

	if len(addrs) == 0 {
		return nil, "", errors.New("redis cluster configuration requires at least one address")
	}

	clusterOpts := &redis.ClusterOptions{
		Addrs:    addrs,
		Password: password,
	}
	if username != "" {
		clusterOpts.Username = username
	}
	if tlsConfig != nil {
		clusterOpts.TLSConfig = tlsConfig
	}

	client := redis.NewClusterClient(clusterOpts)
	return client, "cluster:" + strings.Join(addrs, ","), nil
}

//nolint:ireturn // returning redis.UniversalClient keeps client selection flexible.
func newSentinelClient(cfg config.RedisConfig) (redis.UniversalClient, string, error) {
	if len(cfg.SentinelNodes) == 0 {
		return nil, "", errors.New("redis sentinel configuration requires at least one sentinel node")
	}

	opts := &redis.FailoverOptions{
		MasterName:       cfg.SentinelMasterName,
		SentinelAddrs:    cfg.SentinelNodes,
		Password:         cfg.Password,
		SentinelPassword: cfg.SentinelPassword,
		DB:               0,
	}
	client := redis.NewFailoverClient(opts)
	return client, "sentinel:" + cfg.SentinelMasterName, nil
}

//nolint:ireturn // returning redis.UniversalClient keeps client selection flexible.
func newDirectClient(cfg config.RedisConfig) (redis.UniversalClient, string, error) {
	uri := strings.TrimSpace(cfg.URI)
	if uri == "" {
		return nil, "", errors.New("redis direct configuration requires a URI")
	}

	if isRedisURL(uri) {
		opt, err := redis.ParseURL(uri)
		if err != nil {
			return nil, "", fmt.Errorf("parse redis url: %w", err)
		}
		return redis.NewClient(opt), opt.Addr, nil
	}

	opts := &redis.Options{
		Addr:     uri,
		Password: cfg.Password,
		DB:       0,
	}
	return redis.NewClient(opts), uri, nil
}

func normalizeAddrs(raw []string) []string {
	result := make([]string, 0, len(raw))
	for _, addr := range raw {
		if trimmed := strings.TrimSpace(addr); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func clusterFallbackFromURI(uri, defaultPassword string) (string, string, string, *tls.Config, error) {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return "", "", defaultPassword, nil, nil
	}

	if !isRedisURL(trimmed) {
		return trimmed, "", defaultPassword, nil, nil
	}

	opt, err := redis.ParseURL(trimmed)
	if err != nil {
		return "", "", defaultPassword, nil, fmt.Errorf("parse redis cluster url: %w", err)
	}

	password := defaultPassword
	if opt.Password != "" {
		password = opt.Password
	}

	return opt.Addr, opt.Username, password, opt.TLSConfig, nil
}

func isRedisURL(value string) bool {
	return strings.HasPrefix(value, "redis://") || strings.HasPrefix(value, "rediss://")
}

// RunMigrations runs database migrations.
func RunMigrations(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if err := data.RunMigrations(ctx, db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	if logger != nil {
		logger.InfoContext(ctx, "database migrations completed")
	}

	return nil
}
