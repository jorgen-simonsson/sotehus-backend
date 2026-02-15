package ffr

import (
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

func TestNewService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	stateManager := state.NewManager()

	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &config.Config{
				MQTTBrokerHost: "localhost",
				MQTTBrokerPort: 1883,
			},
			wantErr: false,
		},
		{
			name: "missing broker host",
			cfg: &config.Config{
				MQTTBrokerHost: "",
				MQTTBrokerPort: 1883,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(tt.cfg, stateManager, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewService() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFrequencyParsing(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantFreq  float64
		wantValid bool
	}{
		{
			name:      "normal frequency 50.01",
			payload:   "5001",
			wantFreq:  50.01,
			wantValid: true,
		},
		{
			name:      "normal frequency 50.00",
			payload:   "5000",
			wantFreq:  50.00,
			wantValid: true,
		},
		{
			name:      "low frequency 49.90",
			payload:   "4990",
			wantFreq:  49.90,
			wantValid: true,
		},
		{
			name:      "high frequency 50.10",
			payload:   "5010",
			wantFreq:  50.10,
			wantValid: true,
		},
		{
			name:      "frequency too low (out of range)",
			payload:   "4700",
			wantFreq:  47.00,
			wantValid: false,
		},
		{
			name:      "frequency too high (out of range)",
			payload:   "5300",
			wantFreq:  53.00,
			wantValid: false,
		},
		{
			name:      "invalid payload",
			payload:   "invalid",
			wantFreq:  0,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateManager := state.NewManager()

			freq, valid := parseFrequency(tt.payload)

			if valid != tt.wantValid {
				t.Errorf("parseFrequency() valid = %v, want %v", valid, tt.wantValid)
			}
			if valid && freq != tt.wantFreq {
				t.Errorf("parseFrequency() = %v, want %v", freq, tt.wantFreq)
			}

			if valid {
				stateManager.UpdateFrequency(freq)
				freqData := stateManager.GetFrequencyData()
				if !freqData.Valid {
					t.Error("frequency should be valid after update")
				}
				if freqData.Frequency != tt.wantFreq {
					t.Errorf("frequency = %v, want %v", freqData.Frequency, tt.wantFreq)
				}
			}
		})
	}
}

// parseFrequency is a helper to test the parsing logic
func parseFrequency(payload string) (float64, bool) {
	var rawValue int64
	_, err := fmt.Sscanf(payload, "%d", &rawValue)
	if err != nil {
		return 0, false
	}

	frequency := float64(rawValue) / 100.0

	if frequency < 48.0 || frequency > 52.0 {
		return frequency, false
	}

	return frequency, true
}
