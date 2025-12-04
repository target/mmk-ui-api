package config

import "time"

// DBConfig contains PostgreSQL database configuration.
type DBConfig struct {
	Host     string `env:"HOST"                    envDefault:"localhost"`
	Port     int    `env:"PORT"                    envDefault:"5432"`
	User     string `env:"USER"                    envDefault:"merrymaker"`
	Password string `env:"PASSWORD"                envDefault:"merrymaker"`
	Name     string `env:"NAME"                    envDefault:"merrymaker"`
	SSLMode  string `env:"SSL_MODE"                envDefault:"disable"` // Use 'disable' for local dev, 'require' for production
	// RunMigrationsOnStart controls whether the application automatically applies migrations during startup.
	RunMigrationsOnStart bool `env:"RUN_MIGRATIONS_ON_START" envDefault:"true"`
}

// RedisConfig contains Redis configuration.
type RedisConfig struct {
	URI                string   `env:"URI"                  envDefault:"localhost:6379"`
	Password           string   `env:"PASSWORD"             envDefault:""`
	SentinelPort       string   `env:"SENTINEL_PORT"        envDefault:"26379"`
	SentinelNodes      []string `env:"SENTINEL_NODES"       envDefault:"localhost:26379"`
	SentinelMasterName string   `env:"SENTINEL_MASTER_NAME" envDefault:"mymaster"`
	SentinelPassword   string   `env:"SENTINEL_PASSWORD"    envDefault:""`
	UseSentinel        bool     `env:"USE_SENTINEL"         envDefault:"false"`
	ClusterNodes       []string `env:"CLUSTER_NODES"        envDefault:""`
	UseCluster         bool     `env:"USE_CLUSTER"          envDefault:"false"`
}

// CacheConfig contains cache configuration (Redis-based).
type CacheConfig struct {
	// Redis connection settings for cache.
	RedisAddr     string `env:"CACHE_REDIS_ADDR"     envDefault:"localhost:6379"`
	RedisPassword string `env:"CACHE_REDIS_PASSWORD" envDefault:""`
	RedisDB       int    `env:"CACHE_REDIS_DB"       envDefault:"0"`

	// SourceTTL is the TTL for cached source content.
	SourceTTL time.Duration `env:"CACHE_SOURCE_TTL" envDefault:"30m"`
}
