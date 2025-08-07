package redisreplica

import (
	"time"
)

// loggerAdapter adapts our Logger interface to replication.Logger
type loggerAdapter struct {
	logger Logger
}

func (la *loggerAdapter) Debug(msg string, fields ...interface{}) {
	la.logger.Debug(msg, convertFields(fields...)...)
}

func (la *loggerAdapter) Info(msg string, fields ...interface{}) {
	la.logger.Info(msg, convertFields(fields...)...)
}

func (la *loggerAdapter) Error(msg string, fields ...interface{}) {
	la.logger.Error(msg, convertFields(fields...)...)
}

func convertFields(fields ...interface{}) []Field {
	result := make([]Field, 0, len(fields)/2)
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok {
			result = append(result, Field{
				Key:   key,
				Value: fields[i+1],
			})
		}
	}
	return result
}

// metricsAdapter adapts our MetricsCollector to replication.MetricsCollector
type metricsAdapter struct {
	metrics MetricsCollector
}

func (ma *metricsAdapter) RecordSyncDuration(duration time.Duration) {
	ma.metrics.RecordSyncDuration(duration)
}

func (ma *metricsAdapter) RecordCommandProcessed(cmd string, duration time.Duration) {
	ma.metrics.RecordCommandProcessed(cmd, duration)
}

func (ma *metricsAdapter) RecordNetworkBytes(bytes int64) {
	ma.metrics.RecordNetworkBytes(bytes)
}

func (ma *metricsAdapter) RecordReconnection() {
	ma.metrics.RecordReconnection()
}

func (ma *metricsAdapter) RecordError(errorType string) {
	ma.metrics.RecordError(errorType)
}

// replicationLogger implements replication.Logger interface
type replicationLogger struct {
	logger Logger
}

func (rl *replicationLogger) Debug(msg string, fields ...interface{}) {
	rl.logger.Debug(msg, convertFields(fields...)...)
}

func (rl *replicationLogger) Info(msg string, fields ...interface{}) {
	rl.logger.Info(msg, convertFields(fields...)...)
}

func (rl *replicationLogger) Error(msg string, fields ...interface{}) {
	rl.logger.Error(msg, convertFields(fields...)...)
}
