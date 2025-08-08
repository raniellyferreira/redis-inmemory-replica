package redisreplica

// Version is the current version of the redis-inmemory-replica library.
const Version = "1.1.0"

// GitCommit is the git commit hash (set by build flags)
var GitCommit string

// BuildTime is the build timestamp (set by build flags)
var BuildTime string

// VersionInfo returns detailed version information
func VersionInfo() map[string]string {
	info := map[string]string{
		"version": Version,
	}

	if GitCommit != "" {
		info["commit"] = GitCommit
	}

	if BuildTime != "" {
		info["buildTime"] = BuildTime
	}

	return info
}
