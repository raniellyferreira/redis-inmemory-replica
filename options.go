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
	replicaAddr     string
	replicaPassword string

	// Database selection
	databases       []int // Which databases to replicate (empty = all)
	defaultDatabase int   // Default database to start with (0-15)

	// Timeouts and limits
	syncTimeout       time.Duration
	connectTimeout    time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	heartbeatInterval time.Duration
	maxMemory         int64

	// Observability
	logger  Logger
	metrics MetricsCollector

	// Behavioral options
	readOnly       bool
	commandFilters []string
	redirectWrites bool // Whether to redirect write commands to master instead of returning READONLY error
	
	// Storage options
	shardCount int // Number of shards for storage (0 = use default)
}

// defaultConfig returns a configuration with sensible defaults
func defaultConfig() *config {
	return &config{
		masterAddr:        "localhost:6379",
		replicaAddr:       "",    // No default replica address
		databases:         []int{}, // empty = replicate all databases
		defaultDatabase:   0,     // Default to database 0
		syncTimeout:       30 * time.Second,
		connectTimeout:    5 * time.Second,
		readTimeout:       30 * time.Second,
		writeTimeout:      10 * time.Second,
		heartbeatInterval: 30 * time.Second, // Send REPLCONF ACK every 30 seconds
		maxMemory:         0,                 // unlimited
		readOnly:          true,
		redirectWrites:    false, // Default: return READONLY error for writes
		shardCount:        0,     // 0 = use storage default (64)
		logger:            &defaultLogger{},
		commandFilters:    []string{},
	}
}

// Option represents a configuration option for a Replica
type Option func(*config) error

// WithMaster sets the master Redis server address
//
// Example:
//
//	WithMaster("redis.example.com:6379")
//	WithMaster("localhost:6379")
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
//
//	WithMasterAuth("mypassword")
func WithMasterAuth(password string) Option {
	return func(c *config) error {
		c.masterPassword = password
		return nil
	}
}

// WithReplicaAddr sets the local replica server address
//
// Example:
//
//	WithReplicaAddr(":6380")
//	WithReplicaAddr("0.0.0.0:6380")
func WithReplicaAddr(addr string) Option {
	return func(c *config) error {
		c.replicaAddr = addr
		return nil
	}
}

// WithSyncTimeout sets the initial synchronization timeout
//
// Example:
//
//	WithSyncTimeout(60 * time.Second)
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
//
//	WithConnectTimeout(10 * time.Second)
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
//
//	WithReadTimeout(30 * time.Second)
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
//
//	WithWriteTimeout(10 * time.Second)
func WithWriteTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return ErrInvalidConfig
		}
		c.writeTimeout = timeout
		return nil
	}
}

// WithHeartbeatInterval sets the interval for sending REPLCONF ACK during streaming
// This prevents Redis master from timing out the replica during idle periods
// Set to 0 to use default (30s), or negative value to disable heartbeat
//
// Example:
//
//	WithHeartbeatInterval(30 * time.Second)
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(c *config) error {
		c.heartbeatInterval = interval
		return nil
	}
}

// WithMaxMemory sets the maximum memory usage limit in bytes
// When set to 0, no limit is enforced
//
// Example:
//
//	WithMaxMemory(1024 * 1024 * 1024) // 1GB limit
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
//
//	WithLogger(myCustomLogger)
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
//
//	WithMetrics(myMetricsCollector)
func WithMetrics(collector MetricsCollector) Option {
	return func(c *config) error {
		c.metrics = collector
		return nil
	}
}

// WithTLS configures TLS for the master connection
//
// Example:
//
//	config := &tls.Config{
//	  ServerName: "redis.example.com",
//	}
//	WithTLS(config)
func WithTLS(tlsConfig *tls.Config) Option {
	return func(c *config) error {
		c.masterTLS = tlsConfig
		return nil
	}
}

// TLSEnvironment represents predefined TLS configuration environments
type TLSEnvironment int

const (
	// TLSEnvironmentDevelopment provides relaxed TLS settings for development
	TLSEnvironmentDevelopment TLSEnvironment = iota
	// TLSEnvironmentStaging provides balanced TLS settings for staging
	TLSEnvironmentStaging
	// TLSEnvironmentProduction provides maximum security TLS settings for production
	TLSEnvironmentProduction
)

// WithSecureTLS configures TLS with secure defaults for master connection
//
// This function provides secure TLS defaults and should be used in production
// environments. It enforces certificate verification and uses secure protocols.
//
// Example:
//
//	WithSecureTLS("redis.example.com")
func WithSecureTLS(serverName string) Option {
	return WithTLSEnvironment(serverName, TLSEnvironmentProduction)
}

// WithTLSEnvironment configures TLS with predefined settings for specific environments
//
// This function allows easy configuration switching between development, staging,
// and production environments with appropriate security settings for each.
//
// Examples:
//
//	WithTLSEnvironment("redis.example.com", TLSEnvironmentProduction)  // Maximum security
//	WithTLSEnvironment("redis.staging.com", TLSEnvironmentStaging)     // Balanced security
//	WithTLSEnvironment("localhost", TLSEnvironmentDevelopment)         // Development friendly
func WithTLSEnvironment(serverName string, env TLSEnvironment) Option {
	return func(c *config) error {
		if serverName == "" {
			return ErrInvalidConfig
		}

		var tlsConfig *tls.Config

		switch env {
		case TLSEnvironmentDevelopment:
			// Development: Relaxed settings for easier development
			tlsConfig = &tls.Config{
				ServerName:         serverName,
				InsecureSkipVerify: false, // Still verify certificates for security
				MinVersion:         tls.VersionTLS12,
				// Use broader cipher suite support for compatibility
				CipherSuites: []uint16{
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				},
			}

		case TLSEnvironmentStaging:
			// Staging: Balanced settings for testing production-like security
			tlsConfig = &tls.Config{
				ServerName:         serverName,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
				CipherSuites: []uint16{
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				},
			}

		case TLSEnvironmentProduction:
			// Production: Maximum security settings
			tlsConfig = &tls.Config{
				ServerName:         serverName,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS13, // Require TLS 1.3 for production
				CipherSuites: []uint16{
					// Only the most secure cipher suites
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				},
			}

		default:
			return ErrInvalidConfig
		}

		c.masterTLS = tlsConfig
		return nil
	}
}

// WithTLSCustom configures TLS with custom settings and easy modification
//
// This function provides a builder pattern for custom TLS configuration
// with sensible defaults that can be easily modified.
//
// Example:
//
//	WithTLSCustom("redis.example.com").
//	  MinVersion(tls.VersionTLS13).
//	  SkipVerify(false).
//	  CipherSuites([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384})
func WithTLSCustom(serverName string) *TLSBuilder {
	return &TLSBuilder{
		serverName:         serverName,
		insecureSkipVerify: false,
		minVersion:         tls.VersionTLS12,
		cipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
	}
}

// TLSBuilder provides a fluent interface for building TLS configurations
type TLSBuilder struct {
	serverName         string
	insecureSkipVerify bool
	minVersion         uint16
	cipherSuites       []uint16
}

// MinVersion sets the minimum TLS version
func (b *TLSBuilder) MinVersion(version uint16) *TLSBuilder {
	b.minVersion = version
	return b
}

// SkipVerify sets whether to skip certificate verification (use with caution)
func (b *TLSBuilder) SkipVerify(skip bool) *TLSBuilder {
	b.insecureSkipVerify = skip
	return b
}

// CipherSuites sets the allowed cipher suites
func (b *TLSBuilder) CipherSuites(suites []uint16) *TLSBuilder {
	b.cipherSuites = suites
	return b
}

// Build creates the Option with the configured TLS settings
func (b *TLSBuilder) Build() Option {
	return func(c *config) error {
		if b.serverName == "" {
			return ErrInvalidConfig
		}
		c.masterTLS = &tls.Config{
			ServerName:         b.serverName,
			InsecureSkipVerify: b.insecureSkipVerify,
			MinVersion:         b.minVersion,
			CipherSuites:       b.cipherSuites,
		}
		return nil
	}
}

// WithReadOnly sets whether the replica should be read-only (default: true)
//
// Example:
//
//	WithReadOnly(false) // Allow write operations (dangerous)
func WithReadOnly(readOnly bool) Option {
	return func(c *config) error {
		c.readOnly = readOnly
		return nil
	}
}



// WithCommandFilters sets which commands to replicate
// Empty slice means replicate all commands
//
// Example:
//
//	WithCommandFilters([]string{"SET", "DEL", "EXPIRE"})
func WithCommandFilters(commands []string) Option {
	return func(c *config) error {
		c.commandFilters = append([]string(nil), commands...)
		return nil
	}
}

// WithDatabases sets which databases to replicate
// Empty slice means replicate all databases (default)
//
// Example:
//
//	WithDatabases([]int{0, 1, 2}) // Only replicate databases 0, 1, and 2
//	WithDatabases([]int{0})       // Only replicate database 0
func WithDatabases(databases []int) Option {
	return func(c *config) error {
		// Validate database numbers
		for _, db := range databases {
			if db < 0 || db > 15 {
				return ErrInvalidConfig
			}
		}
		c.databases = append([]int(nil), databases...)
		return nil
	}
}

// WithReplicaAuth sets authentication password for the replica server
// Clients connecting to the replica will need to authenticate with this password
//
// Example:
//
//	WithReplicaAuth("replica-password")
func WithReplicaAuth(password string) Option {
	return func(c *config) error {
		c.replicaPassword = password
		return nil
	}
}

// WithWriteRedirection enables redirection of write commands to the master
// instead of returning READONLY errors (default: false)
// 
// When enabled, write commands like SET, DEL, etc. will be forwarded to the master
// and the response will be returned to the client. When disabled (default), 
// write commands return "READONLY You can't write against a read only replica"
//
// Example:
//
//	WithWriteRedirection(true)  // Enable write redirection
//	WithWriteRedirection(false) // Disable write redirection (default)
func WithWriteRedirection(enabled bool) Option {
	return func(c *config) error {
		c.redirectWrites = enabled
		return nil
	}
}

// WithDefaultDatabase sets the default database to start with (0-15)
// This determines which database clients will initially connect to
//
// Example:
//
//	WithDefaultDatabase(0)  // Start with database 0 (default)
//	WithDefaultDatabase(1)  // Start with database 1
//	WithDefaultDatabase(15) // Start with database 15
func WithDefaultDatabase(db int) Option {
	return func(c *config) error {
		if db < 0 || db > 15 {
			return ErrInvalidConfig
		}
		c.defaultDatabase = db
		return nil
	}
}

// WithShardCount sets the number of shards for the in-memory storage backend
// Sharding reduces lock contention under high concurrency by partitioning data
// across multiple independent shards, each with its own lock
//
// The count is automatically rounded up to the next power of 2 for optimal performance
// Set to 0 to use the storage default (64 shards)
//
// Example:
//
//	WithShardCount(64)   // Use 64 shards (good default)
//	WithShardCount(128)  // Use 128 shards (high concurrency)
//	WithShardCount(32)   // Use 32 shards (lower overhead)
//	WithShardCount(100)  // Rounds up to 128 shards
func WithShardCount(count int) Option {
	return func(c *config) error {
		if count < 0 {
			return ErrInvalidConfig
		}
		c.shardCount = count
		return nil
	}
}
