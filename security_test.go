package redisreplica

import (
	"crypto/tls"
	"testing"
	"time"
)

// TestSecureTLSConfiguration tests the WithSecureTLS option
func TestSecureTLSConfiguration(t *testing.T) {
	cfg := defaultConfig()

	// Test WithSecureTLS
	option := WithSecureTLS("redis.example.com")
	err := option(cfg)
	if err != nil {
		t.Fatalf("WithSecureTLS failed: %v", err)
	}

	// Verify TLS configuration
	if cfg.masterTLS == nil {
		t.Fatal("TLS config should not be nil")
	}

	// Check ServerName
	if cfg.masterTLS.ServerName != "redis.example.com" {
		t.Errorf("Expected ServerName to be 'redis.example.com', got %s", cfg.masterTLS.ServerName)
	}

	// Check InsecureSkipVerify is false
	if cfg.masterTLS.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false for secure TLS")
	}

	// Check MinVersion is TLS 1.3 (production environment default)
	if cfg.masterTLS.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected MinVersion to be TLS 1.3, got %d", cfg.masterTLS.MinVersion)
	}

	// Check that cipher suites are configured
	if len(cfg.masterTLS.CipherSuites) == 0 {
		t.Error("Cipher suites should be configured")
	}
}

// TestSecureTLSConfigurationInvalidServerName tests WithSecureTLS with invalid input
func TestSecureTLSConfigurationInvalidServerName(t *testing.T) {
	cfg := defaultConfig()

	// Test WithSecureTLS with empty server name
	option := WithSecureTLS("")
	err := option(cfg)
	if err == nil {
		t.Fatal("WithSecureTLS should fail with empty server name")
	}

	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

// TestTimeoutConfigurations tests the timeout configuration options
func TestTimeoutConfigurations(t *testing.T) {
	// Test valid timeouts
	testCases := []struct {
		name     string
		option   Option
		getValue func(*config) time.Duration
		expected time.Duration
	}{
		{
			name:     "ConnectTimeout",
			option:   WithConnectTimeout(10 * time.Second),
			getValue: func(c *config) time.Duration { return c.connectTimeout },
			expected: 10 * time.Second,
		},
		{
			name:     "ReadTimeout",
			option:   WithReadTimeout(60 * time.Second),
			getValue: func(c *config) time.Duration { return c.readTimeout },
			expected: 60 * time.Second,
		},
		{
			name:     "WriteTimeout",
			option:   WithWriteTimeout(15 * time.Second),
			getValue: func(c *config) time.Duration { return c.writeTimeout },
			expected: 15 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			err := tc.option(cfg)
			if err != nil {
				t.Fatalf("Option %s failed: %v", tc.name, err)
			}

			actual := tc.getValue(cfg)
			if actual != tc.expected {
				t.Errorf("Expected %s to be %v, got %v", tc.name, tc.expected, actual)
			}
		})
	}
}

// TestInvalidTimeoutConfigurations tests timeout configuration with invalid values
func TestInvalidTimeoutConfigurations(t *testing.T) {
	testCases := []struct {
		name   string
		option Option
	}{
		{"ConnectTimeout", WithConnectTimeout(0)},
		{"ConnectTimeout negative", WithConnectTimeout(-1 * time.Second)},
		{"ReadTimeout", WithReadTimeout(0)},
		{"ReadTimeout negative", WithReadTimeout(-1 * time.Second)},
		{"WriteTimeout", WithWriteTimeout(0)},
		{"WriteTimeout negative", WithWriteTimeout(-1 * time.Second)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			err := tc.option(cfg)
			if err == nil {
				t.Fatalf("Option %s should fail with invalid timeout", tc.name)
			}

			if err != ErrInvalidConfig {
				t.Errorf("Expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

// TestSecurityDefaults tests that security defaults are appropriate
func TestSecurityDefaults(t *testing.T) {
	cfg := defaultConfig()

	// Check that read-only is true by default
	if !cfg.readOnly {
		t.Error("Default configuration should be read-only")
	}

	// Check that timeouts have reasonable defaults
	if cfg.connectTimeout <= 0 {
		t.Error("Connect timeout should have a positive default")
	}

	if cfg.readTimeout <= 0 {
		t.Error("Read timeout should have a positive default")
	}

	if cfg.writeTimeout <= 0 {
		t.Error("Write timeout should have a positive default")
	}

	// Check that sync timeout has a reasonable default
	if cfg.syncTimeout <= 0 {
		t.Error("Sync timeout should have a positive default")
	}
}

// TestSecureReplicaConfiguration tests a complete secure configuration
func TestSecureReplicaConfiguration(t *testing.T) {
	replica, err := New(
		WithMaster("redis.secure.example.com:6380"),
		WithMasterAuth("secure-password-123"),
		WithSecureTLS("redis.secure.example.com"),
		WithConnectTimeout(10*time.Second),
		WithReadTimeout(30*time.Second),
		WithWriteTimeout(10*time.Second),
		WithMaxMemory(100*1024*1024), // 100MB
		WithReadOnly(true),
		WithDatabases([]int{0, 1}), // Only replicate specific databases
	)

	if err != nil {
		t.Fatalf("Failed to create secure replica: %v", err)
	}

	if replica == nil {
		t.Fatal("Replica should not be nil")
	}

	// Test that the replica was created with secure defaults
	if !replica.config.readOnly {
		t.Error("Replica should be read-only for security")
	}

	if replica.config.masterTLS == nil {
		t.Error("TLS should be configured")
	}

	if replica.config.masterPassword == "" {
		t.Error("Authentication should be configured")
	}

	if len(replica.config.databases) != 2 {
		t.Error("Database filtering should be configured")
	}
}
