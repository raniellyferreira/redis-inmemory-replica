package redisreplica

import (
	"context"
	"testing"
	"time"
)

// TestSecurityIntegration tests the integration of all security features
func TestSecurityIntegration(t *testing.T) {
	// Test that security features work together without errors
	replica, err := New(
		WithMaster("localhost:6379"),
		WithMasterAuth("test-password"),
		WithSecureTLS("localhost"), // Using localhost for test
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
	if config.masterPassword != "test-password" {
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
	// Create replica with very short timeout to test timeout logic
	replica, err := New(
		WithMaster("192.0.2.1:6379"), // Non-routable IP for testing timeout
		WithConnectTimeout(100*time.Millisecond),
		WithReadTimeout(50*time.Millisecond),
		WithWriteTimeout(50*time.Millisecond),
	)
	
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	
	// Test that connection timeout is respected
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	// This should timeout quickly due to non-routable IP
	startTime := time.Now()
	err = replica.Start(ctx)
	elapsed := time.Since(startTime)
	
	// Connection should fail quickly (within a reasonable time)
	if elapsed > 2*time.Second {
		t.Errorf("Connection took too long: %v", elapsed)
	}
	
	// Should get an error (connection failure or timeout)
	if err == nil {
		t.Error("Expected connection error due to non-routable IP")
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
		"InsecureSkipVerify is false":        !tls.InsecureSkipVerify,
		"MinVersion is TLS 1.2":             tls.MinVersion >= 0x0303, // TLS 1.2
		"PreferServerCipherSuites is true":  tls.PreferServerCipherSuites,
		"ServerName is set":                 tls.ServerName != "",
		"CipherSuites are configured":       len(tls.CipherSuites) > 0,
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