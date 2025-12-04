package bootstrap

import (
	"testing"

	"github.com/target/mmk-ui-api/config"
)

func TestErrorChannelCapacity(t *testing.T) {
	tests := []struct {
		name  string
		modes []config.ServiceMode
		want  int
	}{
		{
			name: "no services enabled",
			want: 0,
		},
		{
			name:  "http only",
			modes: []config.ServiceMode{config.ServiceModeHTTP},
			want:  1,
		},
		{
			name:  "http and rules engine",
			modes: []config.ServiceMode{config.ServiceModeHTTP, config.ServiceModeRulesEngine},
			want:  2,
		},
		{
			name:  "secret refresh and alert runner",
			modes: []config.ServiceMode{config.ServiceModeSecretRefreshRunner, config.ServiceModeAlertRunner},
			want:  2,
		},
		{
			name: "all services enabled",
			modes: []config.ServiceMode{
				config.ServiceModeHTTP,
				config.ServiceModeRulesEngine,
				config.ServiceModeScheduler,
				config.ServiceModeAlertRunner,
				config.ServiceModeSecretRefreshRunner,
				config.ServiceModeReaper,
			},
			want: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := make(map[config.ServiceMode]bool, len(tt.modes))
			for _, mode := range tt.modes {
				enabled[mode] = true
			}

			if got := errorChannelCapacity(enabled); got != tt.want {
				t.Fatalf("errorChannelCapacity(%v) = %d, want %d", tt.modes, got, tt.want)
			}
		})
	}
}

func TestErrorChannelBufferSize(t *testing.T) {
	tests := []struct {
		name  string
		modes []config.ServiceMode
		want  int
	}{
		{
			name: "no services enabled",
			want: 1,
		},
		{
			name:  "http only",
			modes: []config.ServiceMode{config.ServiceModeHTTP},
			want:  2,
		},
		{
			name: "all services enabled",
			modes: []config.ServiceMode{
				config.ServiceModeHTTP,
				config.ServiceModeRulesEngine,
				config.ServiceModeScheduler,
				config.ServiceModeAlertRunner,
				config.ServiceModeSecretRefreshRunner,
				config.ServiceModeReaper,
			},
			want: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := make(map[config.ServiceMode]bool, len(tt.modes))
			for _, mode := range tt.modes {
				enabled[mode] = true
			}

			if got := errorChannelBufferSize(enabled); got != tt.want {
				t.Fatalf("errorChannelBufferSize(%v) = %d, want %d", tt.modes, got, tt.want)
			}
		})
	}
}
