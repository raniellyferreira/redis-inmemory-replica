package redisreplica

import (
	"testing"
	"time"
)

func TestHeartbeatConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantErr  bool
	}{
		{
			name:     "default heartbeat interval",
			interval: 0, // should use default
			wantErr:  false,
		},
		{
			name:     "valid heartbeat interval",
			interval: 15 * time.Second,
			wantErr:  false,
		},
		{
			name:     "disabled heartbeat",
			interval: -1,    // disabled
			wantErr:  false, // allowed
		},
		{
			name:     "fast heartbeat",
			interval: 1 * time.Second,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []Option{
				WithMaster("localhost:6379"),
			}

			if tt.interval != 0 {
				opts = append(opts, WithHeartbeatInterval(tt.interval))
			}

			replica, err := New(opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				defer func() {
					if err := replica.Close(); err != nil {
						t.Logf("Failed to close replica: %v", err)
					}
				}()
				// Just verify the replica was created successfully
				if replica == nil {
					t.Error("Expected replica to be created")
				}
			}
		})
	}
}

func TestHeartbeatIntervalValidation(t *testing.T) {
	// Test that negative intervals are allowed (disables heartbeat)
	replica, err := New(
		WithMaster("localhost:6379"),
		WithHeartbeatInterval(-1),
	)
	if err != nil {
		t.Errorf("Expected negative heartbeat interval to be allowed, got error: %v", err)
	}
	if replica != nil {
		if err := replica.Close(); err != nil {
			t.Logf("Failed to close replica: %v", err)
		}
	}
}
