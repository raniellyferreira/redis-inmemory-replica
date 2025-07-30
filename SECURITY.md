# Security Policy

## Reporting Security Vulnerabilities

We take the security of the Redis In-Memory Replica library seriously. If you discover a security vulnerability, please follow responsible disclosure practices.

### How to Report

1. **DO NOT** open a public GitHub issue for security vulnerabilities
2. Send an email to the maintainer with details of the vulnerability
3. Include steps to reproduce the issue and potential impact
4. We will acknowledge receipt within 48 hours

### Security Best Practices for Users

When using this library in production, follow these security guidelines:

## Authentication & Authorization

### Master Authentication
```go
replica, err := redisreplica.New(
    redisreplica.WithMasterAddr("redis.example.com:6379"),
    redisreplica.WithMasterAuth("your-secure-password"),
)
```

### TLS Configuration
Always use TLS in production environments:
```go
tlsConfig := &tls.Config{
    ServerName:         "redis.example.com",
    InsecureSkipVerify: false, // Always verify certificates in production
}

replica, err := redisreplica.New(
    redisreplica.WithMasterAddr("redis.example.com:6380"),
    redisreplica.WithMasterTLS(tlsConfig),
)
```

### Replica Server Security
If enabling the replica server, secure it properly:
```go
replica, err := redisreplica.New(
    redisreplica.WithReplicaAddr("127.0.0.1:6381"), // Bind to specific interface
    redisreplica.WithReplicaAuth("replica-password"), // Require authentication
    redisreplica.WithReadOnly(true), // Enable read-only mode
)
```

## Network Security

### Connection Timeouts
Configure appropriate timeouts to prevent hanging connections:
```go
replica, err := redisreplica.New(
    redisreplica.WithConnectTimeout(5*time.Second),
    redisreplica.WithReadTimeout(30*time.Second),
    redisreplica.WithWriteTimeout(10*time.Second),
)
```

### Database Filtering
Limit replication to specific databases:
```go
replica, err := redisreplica.New(
    redisreplica.WithDatabases([]int{0, 1}), // Only replicate databases 0 and 1
)
```

## Memory Management

### Memory Limits
Set memory limits to prevent DoS attacks:
```go
replica, err := redisreplica.New(
    redisreplica.WithMaxMemory(100*1024*1024), // 100MB limit
)
```

## Monitoring & Logging

### Secure Logging
- Avoid logging sensitive data like passwords or keys
- Use structured logging with appropriate log levels
- Implement log rotation and secure storage

```go
replica, err := redisreplica.New(
    redisreplica.WithLogger(&secureLogger{
        // Your secure logger implementation
    }),
)
```

### Monitoring
Monitor for suspicious activities:
- Connection failures and retry patterns
- Authentication failures
- Memory usage spikes
- Unexpected disconnections

## Deployment Security

### Process Isolation
- Run the replica process with minimal privileges
- Use container security features (seccomp, AppArmor, etc.)
- Implement proper resource limits

### Network Isolation
- Use firewalls to restrict access to Redis ports
- Consider VPN or private networks for master-replica communication
- Implement proper network segmentation

### Secrets Management
- Use secure secret management systems (HashiCorp Vault, AWS Secrets Manager, etc.)
- Avoid hardcoding credentials in configuration files
- Rotate credentials regularly

## Security Checklist

Before deploying to production:

- [ ] TLS enabled and properly configured
- [ ] Strong authentication credentials
- [ ] Network access properly restricted
- [ ] Memory limits configured
- [ ] Logging configured securely
- [ ] Monitoring and alerting in place
- [ ] Regular security updates applied
- [ ] Vulnerability scanning integrated into CI/CD

## Known Security Considerations

### Current Limitations
1. Passwords are stored in memory as plain strings (common for client libraries)
2. No built-in rate limiting for authentication attempts
3. Default TLS configuration may need hardening for high-security environments

### Mitigation Strategies
1. Use secure memory management practices at the application level
2. Implement application-level rate limiting if needed
3. Configure TLS with strong cipher suites and proper certificate validation

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| Latest  | :white_check_mark: |
| Others  | :x:                |

We only provide security updates for the latest version. Please keep your dependencies up to date.

## Security Testing

This project includes automated security testing:
- Static analysis with gosec
- Vulnerability scanning with govulncheck
- Dependency vulnerability checks
- Code quality analysis

Tests run automatically on every pull request and are scheduled weekly.