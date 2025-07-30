package redisreplica

import (
	"crypto/tls"
	"time"
)

// config holds the configuration for a Replica
type config struct {
	// Master connection settings
	masterAddr     string
	masterPassword string
	masterTLS      *tls.Config
	
	// Replica server settings
	replicaAddr string
	
	// Timeouts and limits
	syncTimeout      time.Duration
	connectTimeout   time.Duration
	readTimeout      time.Duration
	writeTimeout     time.Duration
	maxMemory        int64
	
	// Observability
	logger    Logger
	metrics   MetricsCollector
	
	// Behavioral options
	readOnly       bool
	enableServer   bool
	commandFilters []string
}

// defaultConfig returns a configuration with sensible defaults
func defaultConfig() *config {
	return &config{
		masterAddr:     "localhost:6379",
		replicaAddr:    ":6380",
		syncTimeout:    30 * time.Second,
		connectTimeout: 5 * time.Second,
		readTimeout:    30 * time.Second,
		writeTimeout:   10 * time.Second,
		maxMemory:      0, // unlimited
		readOnly:       true,
		enableServer:   true,
		logger:         &defaultLogger{},
		commandFilters: []string{},
	}
}

// Option represents a configuration option for a Replica
type Option func(*config) error

// WithMaster sets the master Redis server address
//
// Example:
//   WithMaster("redis.example.com:6379")
//   WithMaster("localhost:6379")
func WithMaster(addr string) Option {
	return func(c *config) error {
		if addr == "" {
			return &ConnectionError{
				Addr: addr,
				Err:  ErrInvalidConfig,
			}
		}
		c.masterAddr = addr
		return nil
	}
}

// WithMasterAuth sets authentication credentials for the master connection
//
// Example:
//   WithMasterAuth("mypassword")
func WithMasterAuth(password string) Option {
	return func(c *config) error {
		c.masterPassword = password
		return nil
	}
}

// WithReplicaAddr sets the local replica server address
//
// Example:
//   WithReplicaAddr(":6380")
//   WithReplicaAddr("0.0.0.0:6380")
func WithReplicaAddr(addr string) Option {
	return func(c *config) error {
		c.replicaAddr = addr
		return nil
	}
}

// WithSyncTimeout sets the initial synchronization timeout
//
// Example:
//   WithSyncTimeout(60 * time.Second)
func WithSyncTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return ErrInvalidConfig
		}
		c.syncTimeout = timeout
		return nil
	}
}

// WithConnectTimeout sets the connection timeout for master connection
//
// Example:
//   WithConnectTimeout(10 * time.Second)
func WithConnectTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return ErrInvalidConfig
		}
		c.connectTimeout = timeout
		return nil
	}
}

// WithReadTimeout sets the read timeout for network operations
//
// Example:
//   WithReadTimeout(30 * time.Second)
func WithReadTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return ErrInvalidConfig
		}
		c.readTimeout = timeout
		return nil
	}
}

// WithWriteTimeout sets the write timeout for network operations
//
// Example:
//   WithWriteTimeout(10 * time.Second)
func WithWriteTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return ErrInvalidConfig
		}
		c.writeTimeout = timeout
		return nil
	}
}

// WithMaxMemory sets the maximum memory usage limit in bytes
// When set to 0, no limit is enforced
//
// Example:
//   WithMaxMemory(1024 * 1024 * 1024) // 1GB limit
func WithMaxMemory(bytes int64) Option {
	return func(c *config) error {
		if bytes < 0 {
			return ErrInvalidConfig
		}
		c.maxMemory = bytes
		return nil
	}
}

// WithLogger sets a custom logger for the replica
//
// Example:
//   WithLogger(myCustomLogger)
func WithLogger(logger Logger) Option {
	return func(c *config) error {
		if logger == nil {
			return ErrInvalidConfig
		}
		c.logger = logger
		return nil
	}
}

// WithMetrics enables metrics collection with the provided collector
//
// Example:
//   WithMetrics(myMetricsCollector)
func WithMetrics(collector MetricsCollector) Option {
	return func(c *config) error {
		c.metrics = collector
		return nil
	}
}

// WithTLS configures TLS for the master connection
//
// Example:
//   config := &tls.Config{
//     ServerName: "redis.example.com",
//   }
//   WithTLS(config)
func WithTLS(tlsConfig *tls.Config) Option {
	return func(c *config) error {
		c.masterTLS = tlsConfig
		return nil
	}
}

// WithReadOnly sets whether the replica should be read-only (default: true)
//
// Example:
//   WithReadOnly(false) // Allow write operations (dangerous)
func WithReadOnly(readOnly bool) Option {
	return func(c *config) error {
		c.readOnly = readOnly
		return nil
	}
}

// WithServerEnabled controls whether to start the Redis-compatible server
//
// Example:
//   WithServerEnabled(false) // Disable server, use only as library
func WithServerEnabled(enabled bool) Option {
	return func(c *config) error {
		c.enableServer = enabled
		return nil
	}
}

// WithCommandFilters sets which commands to replicate
// Empty slice means replicate all commands
//
// Example:
//   WithCommandFilters([]string{"SET", "DEL", "EXPIRE"})
func WithCommandFilters(commands []string) Option {
	return func(c *config) error {
		c.commandFilters = append([]string(nil), commands...)
		return nil
	}
}