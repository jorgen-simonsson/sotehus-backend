package state

import (
	"sync"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
)

// Manager provides thread-safe access to application state
type Manager struct {
	mu sync.RWMutex

	gridData      models.GridData
	priceData     models.PriceData
	solarData     models.SolarData
	frequencyData models.FrequencyData

	// Frequency accumulator for computing average between InfluxDB writes
	freqSum        float64
	freqCount      int
	freqLastUpdate time.Time
}

// NewManager creates a new state manager with default values
func NewManager() *Manager {
	return &Manager{
		gridData: models.GridData{
			Valid:   false,
			Message: "No data received yet",
		},
		priceData: models.PriceData{
			Valid: false,
		},
		solarData: models.SolarData{
			Valid: false,
		},
		frequencyData: models.FrequencyData{
			Valid: false,
		},
	}
}

// UpdateGrid updates the grid power data with the given timestamp
func (m *Manager) UpdateGrid(power float64, timestamp time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.gridData = models.GridData{
		Valid:      true,
		Power:      power,
		LastUpdate: timestamp,
		Message:    "",
	}
}

// UpdateGridError marks grid data as invalid with an error message
func (m *Manager) UpdateGridError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.gridData.Valid = false
	m.gridData.Message = message
}

// UpdatePrice updates the spot price data
func (m *Manager) UpdatePrice(price float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.priceData = models.PriceData{
		Valid:      true,
		Price:      price,
		LastUpdate: time.Now(),
	}
}

// UpdatePriceError marks price data as invalid
func (m *Manager) UpdatePriceError() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.priceData.Valid = false
}

// UpdateSolar updates the solar power data
func (m *Manager) UpdateSolar(power float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.solarData = models.SolarData{
		Valid:      true,
		Power:      power,
		LastUpdate: time.Now(),
		Message:    "",
	}
}

// UpdateSolarError marks solar data as invalid with a message
func (m *Manager) UpdateSolarError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.solarData.Valid = false
	m.solarData.Message = message
}

// GetAPIResponse returns the current state as an API response
func (m *Manager) GetAPIResponse() models.APIResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return models.APIResponse{
		Grid:      m.gridData,
		Price:     m.priceData,
		Solar:     m.solarData,
		Frequency: m.frequencyData,
	}
}

// GetGridData returns a copy of the current grid data
func (m *Manager) GetGridData() models.GridData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gridData
}

// GetPriceData returns a copy of the current price data
func (m *Manager) GetPriceData() models.PriceData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.priceData
}

// GetSolarData returns a copy of the current solar data
func (m *Manager) GetSolarData() models.SolarData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.solarData
}

// UpdateFrequency accumulates a frequency sample for averaging.
// The API-visible frequencyData is only updated when GetAndResetAverageFrequency is called.
func (m *Manager) UpdateFrequency(frequency float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.freqSum += frequency
	m.freqCount++
	m.freqLastUpdate = time.Now()
}

// GetAndResetAverageFrequency computes the average frequency since the last
// reset, updates the API-visible frequencyData, resets the accumulator, and
// returns the averaged FrequencyData. Returns Invalid if no samples exist.
func (m *Manager) GetAndResetAverageFrequency() models.FrequencyData {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.freqCount == 0 {
		return models.FrequencyData{Valid: false}
	}

	avg := m.freqSum / float64(m.freqCount)
	m.freqSum = 0
	m.freqCount = 0

	m.frequencyData = models.FrequencyData{
		Valid:      true,
		Frequency:  avg,
		LastUpdate: m.freqLastUpdate,
	}

	return m.frequencyData
}

// GetFrequencyData returns a copy of the current frequency data
func (m *Manager) GetFrequencyData() models.FrequencyData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.frequencyData
}
