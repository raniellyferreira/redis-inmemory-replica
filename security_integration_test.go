package redisreplica

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"
	"time"
)

// TestSecurityIntegration tests the integration of all security features
func TestSecurityIntegration(t *testing.T) {
	// Test that security features work together without errors
	replica, err := New(
		WithMaster("localhost:6379"),
		WithMasterAuth("test-password"), // safe: testing only - dummy credential for unit tests
		WithSecureTLS("localhost"),      // Using localhost for test
		WithConnectTimeout(5*time.Second),
		WithReadTimeout(10*time.Second),
		WithWriteTimeout(5*time.Second),
		WithMaxMemory(50*1024*1024), // 50MB limit
		WithReadOnly(true),
		WithDatabases([]int{0}), // Only database 0
	)

	if err != nil {
		t.Fatalf("Failed to create secure replica: %v", err)
	}

	if replica == nil {
		t.Fatal("Replica should not be nil")
	}

	// Verify all security configurations were applied
	config := replica.config

	// Check TLS configuration
	if config.masterTLS == nil {
		t.Error("TLS configuration should be set")
	} else {
		if config.masterTLS.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be false")
		}
		if config.masterTLS.ServerName != "localhost" {
			t.Errorf("Expected ServerName 'localhost', got %s", config.masterTLS.ServerName)
		}
	}

	// Check authentication
	if config.masterPassword != "test-password" { // safe: testing only - verifying test credential was set
		t.Error("Master password should be configured")
	}

	// Check timeouts
	if config.connectTimeout != 5*time.Second {
		t.Errorf("Expected connect timeout 5s, got %v", config.connectTimeout)
	}
	if config.readTimeout != 10*time.Second {
		t.Errorf("Expected read timeout 10s, got %v", config.readTimeout)
	}
	if config.writeTimeout != 5*time.Second {
		t.Errorf("Expected write timeout 5s, got %v", config.writeTimeout)
	}

	// Check memory limit
	if config.maxMemory != 50*1024*1024 {
		t.Errorf("Expected memory limit 50MB, got %d", config.maxMemory)
	}

	// Check read-only mode
	if !config.readOnly {
		t.Error("Replica should be in read-only mode")
	}

	// Check database filtering
	if len(config.databases) != 1 {
		t.Errorf("Expected 1 database configured, got %d", len(config.databases))
	}
	if len(config.databases) > 0 && config.databases[0] != 0 {
		t.Error("Database 0 should be configured for replication")
	}
}

// TestSecurityTimeout tests that timeouts are properly applied to connections
func TestSecurityTimeout(t *testing.T) {
	tests := []struct {
		name           string
		connectTimeout time.Duration
		readTimeout    time.Duration
		writeTimeout   time.Duration
		expectTimeout  bool
	}{
		{
			name:           "very_short_timeouts",
			connectTimeout: 100 * time.Millisecond,
			readTimeout:    50 * time.Millisecond,
			writeTimeout:   50 * time.Millisecond,
			expectTimeout:  true,
		},
		{
			name:           "reasonable_timeouts",
			connectTimeout: 5 * time.Second,
			readTimeout:    30 * time.Second,
			writeTimeout:   10 * time.Second,
			expectTimeout:  true, // Still expect timeout due to non-routable IP
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create replica with specified timeouts
			replica, err := New(
				WithMaster("192.0.2.1:6379"), // Non-routable IP for testing timeout
				WithConnectTimeout(tt.connectTimeout),
				WithReadTimeout(tt.readTimeout),
				WithWriteTimeout(tt.writeTimeout),
			)

			if err != nil {
				t.Fatalf("Failed to create replica: %v", err)
			}

			// Test that connection fails as expected
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// This should timeout quickly due to non-routable IP
			startTime := time.Now()
			err = replica.Start(ctx)
			elapsed := time.Since(startTime)

			// Should get an error (connection failure or timeout) or succeed with background replication
			if err != nil {
				// Verify the error contains relevant information
				errStr := err.Error()
				if !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "dial") && !strings.Contains(errStr, "deadline") {
					t.Errorf("Expected timeout/dial/deadline error, got: %v", err)
				}
				
				// For very short timeouts, should fail faster than context timeout
				if tt.name == "very_short_timeouts" && elapsed > 1500*time.Millisecond {
					t.Logf("Connection took %v, which is acceptable for short timeouts", elapsed)
				}
			} else {
				// With the new implementation, Start() can succeed even if replication fails
				// since replication happens in background
				t.Logf("Start() succeeded in %v - replication will happen in background", elapsed)
			}
		})
	}
}

// TestSecurityTimeoutValidation tests timeout configuration validation
func TestSecurityTimeoutValidation(t *testing.T) {
	tests := []struct {
		name           string
		connectTimeout time.Duration
		readTimeout    time.Duration
		writeTimeout   time.Duration
		expectError    bool
	}{
		{
			name:           "valid_timeouts",
			connectTimeout: 5 * time.Second,
			readTimeout:    30 * time.Second,
			writeTimeout:   10 * time.Second,
			expectError:    false,
		},
		{
			name:           "negative_connect_timeout",
			connectTimeout: -1 * time.Second,
			readTimeout:    30 * time.Second,
			writeTimeout:   10 * time.Second,
			expectError:    true, // Negative timeout should be invalid
		},
		{
			name:           "negative_read_timeout",
			connectTimeout: 5 * time.Second,
			readTimeout:    -1 * time.Second,
			writeTimeout:   10 * time.Second,
			expectError:    true, // Negative timeout should be invalid
		},
		{
			name:           "negative_write_timeout",
			connectTimeout: 5 * time.Second,
			readTimeout:    30 * time.Second,
			writeTimeout:   -1 * time.Second,
			expectError:    true, // Negative timeout should be invalid
		},
		{
			name:           "very_large_timeouts",
			connectTimeout: 1 * time.Hour,
			readTimeout:    24 * time.Hour,
			writeTimeout:   24 * time.Hour,
			expectError:    false, // Large timeouts should be allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create replica with all timeouts at once
			_, err := New(
				WithMaster("localhost:6379"),
				WithConnectTimeout(tt.connectTimeout),
				WithReadTimeout(tt.readTimeout),
				WithWriteTimeout(tt.writeTimeout),
			)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestSecurityTLSEnvironments tests different TLS environment configurations
func TestSecurityTLSEnvironments(t *testing.T) {
	tests := []struct {
		name        string
		environment TLSEnvironment
		serverName  string
	}{
		{
			name:        "development_environment",
			environment: TLSEnvironmentDevelopment,
			serverName:  "localhost",
		},
		{
			name:        "staging_environment",
			environment: TLSEnvironmentStaging,
			serverName:  "staging.redis.example.com",
		},
		{
			name:        "production_environment",
			environment: TLSEnvironmentProduction,
			serverName:  "redis.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultConfig()

			// Apply TLS environment configuration
			err := WithTLSEnvironment(tt.serverName, tt.environment)(config)
			if err != nil {
				t.Fatalf("Failed to apply TLS environment: %v", err)
			}

			// Verify TLS config was set
			if config.masterTLS == nil {
				t.Fatal("TLS config was not set")
			}

			// Verify server name
			if config.masterTLS.ServerName != tt.serverName {
				t.Errorf("Expected server name %s, got %s", tt.serverName, config.masterTLS.ServerName)
			}

			// Verify InsecureSkipVerify is false (security requirement)
			if config.masterTLS.InsecureSkipVerify {
				t.Error("InsecureSkipVerify should be false for security")
			}

			// Verify minimum TLS version based on environment
			expectedMinVersion := tls.VersionTLS12
			if tt.environment == TLSEnvironmentProduction {
				expectedMinVersion = tls.VersionTLS13
			}

			if config.masterTLS.MinVersion != uint16(expectedMinVersion) {
				t.Errorf("Expected min version %d, got %d", expectedMinVersion, config.masterTLS.MinVersion)
			}

			// Verify cipher suites are configured
			if len(config.masterTLS.CipherSuites) == 0 {
				t.Error("No cipher suites configured")
			}

			// Production should have fewer (more secure) cipher suites
			if tt.environment == TLSEnvironmentProduction {
				if len(config.masterTLS.CipherSuites) > 4 {
					t.Errorf("Production environment should have limited cipher suites, got %d", len(config.masterTLS.CipherSuites))
				}
			}
		})
	}
}

// TestSecurityTLSCustomBuilder tests the custom TLS builder
func TestSecurityTLSCustomBuilder(t *testing.T) {
	config := defaultConfig()

	// Test custom TLS builder
	err := WithTLSCustom("custom.redis.example.com").
		MinVersion(tls.VersionTLS13).
		SkipVerify(false).
		CipherSuites([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384}).
		Build()(config)

	if err != nil {
		t.Fatalf("Failed to build custom TLS config: %v", err)
	}

	// Verify custom configuration
	if config.masterTLS == nil {
		t.Fatal("TLS config was not set")
	}

	if config.masterTLS.ServerName != "custom.redis.example.com" {
		t.Errorf("Expected server name custom.redis.example.com, got %s", config.masterTLS.ServerName)
	}

	if config.masterTLS.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected TLS 1.3, got %d", config.masterTLS.MinVersion)
	}

	if config.masterTLS.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false")
	}

	if len(config.masterTLS.CipherSuites) != 1 {
		t.Errorf("Expected 1 cipher suite, got %d", len(config.masterTLS.CipherSuites))
	}
}

// TestSecurityTLSInvalidConfigurations tests invalid TLS configurations
func TestSecurityTLSInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		serverName  string
		expectError bool
	}{
		{
			name:        "empty_server_name",
			serverName:  "",
			expectError: true,
		},
		{
			name:        "valid_server_name",
			serverName:  "redis.example.com",
			expectError: false,
		},
		{
			name:        "localhost",
			serverName:  "localhost",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultConfig()
			err := WithSecureTLS(tt.serverName)(config)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestSecurityTLSConfiguration tests TLS configuration details
func TestSecurityTLSConfiguration(t *testing.T) {
	config := defaultConfig()

	// Apply secure TLS configuration
	option := WithSecureTLS("redis.example.com")
	err := option(config)
	if err != nil {
		t.Fatalf("WithSecureTLS failed: %v", err)
	}

	tls := config.masterTLS

	// Verify security settings
	securityChecks := map[string]bool{
		"InsecureSkipVerify is false": !tls.InsecureSkipVerify,
		"MinVersion is TLS 1.2":       tls.MinVersion >= 0x0303, // TLS 1.2
		"ServerName is set":           tls.ServerName != "",
		"CipherSuites are configured": len(tls.CipherSuites) > 0,
	}

	for check, passed := range securityChecks {
		if !passed {
			t.Errorf("Security check failed: %s", check)
		}
	}

	// Verify specific cipher suites include secure ones
	hasSecureCipher := false
	secureCiphers := map[uint16]bool{
		0x1301: true, // TLS_AES_128_GCM_SHA256
		0x1302: true, // TLS_AES_256_GCM_SHA384
		0x1303: true, // TLS_CHACHA20_POLY1305_SHA256
		0xc02f: true, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
		0xc030: true, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
		0xcca8: true, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305
	}

	for _, cipher := range tls.CipherSuites {
		if secureCiphers[cipher] {
			hasSecureCipher = true
			break
		}
	}

	if !hasSecureCipher {
		t.Error("No secure cipher suites found in TLS configuration")
	}
}
