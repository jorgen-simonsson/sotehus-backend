package state

import (
	"sync"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}

	// Check initial state
	grid := m.GetGridData()
	if grid.Valid {
		t.Error("Initial grid.Valid should be false")
	}
	if grid.Message != "No data received yet" {
		t.Errorf("Initial grid.Message = %q, want %q", grid.Message, "No data received yet")
	}

	price := m.GetPriceData()
	if price.Valid {
		t.Error("Initial price.Valid should be false")
	}

	solar := m.GetSolarData()
	if solar.Valid {
		t.Error("Initial solar.Valid should be false")
	}

	frequency := m.GetFrequencyData()
	if frequency.Valid {
		t.Error("Initial frequency.Valid should be false")
	}
}

func TestUpdateGrid(t *testing.T) {
	m := NewManager()
	timestamp := time.Now()

	m.UpdateGrid(1234.5, timestamp)

	grid := m.GetGridData()
	if !grid.Valid {
		t.Error("grid.Valid should be true after update")
	}
	if grid.Power != 1234.5 {
		t.Errorf("grid.Power = %f, want %f", grid.Power, 1234.5)
	}
	if !grid.LastUpdate.Equal(timestamp) {
		t.Errorf("grid.LastUpdate = %v, want %v", grid.LastUpdate, timestamp)
	}
	if grid.Message != "" {
		t.Errorf("grid.Message = %q, want empty string", grid.Message)
	}
}

func TestUpdateGridError(t *testing.T) {
	m := NewManager()
	timestamp := time.Now()

	// First set valid data
	m.UpdateGrid(1234.5, timestamp)

	// Then set error
	m.UpdateGridError("Connection lost")

	grid := m.GetGridData()
	if grid.Valid {
		t.Error("grid.Valid should be false after error")
	}
	if grid.Message != "Connection lost" {
		t.Errorf("grid.Message = %q, want %q", grid.Message, "Connection lost")
	}
}

func TestUpdatePrice(t *testing.T) {
	m := NewManager()

	m.UpdatePrice(1.25)

	price := m.GetPriceData()
	if !price.Valid {
		t.Error("price.Valid should be true after update")
	}
	if price.Price != 1.25 {
		t.Errorf("price.Price = %f, want %f", price.Price, 1.25)
	}
	if price.LastUpdate.IsZero() {
		t.Error("price.LastUpdate should not be zero")
	}
}

func TestUpdatePriceError(t *testing.T) {
	m := NewManager()

	// First set valid data
	m.UpdatePrice(1.25)

	// Then set error
	m.UpdatePriceError()

	price := m.GetPriceData()
	if price.Valid {
		t.Error("price.Valid should be false after error")
	}
}

func TestUpdateSolar(t *testing.T) {
	m := NewManager()

	m.UpdateSolar(5000.0)

	solar := m.GetSolarData()
	if !solar.Valid {
		t.Error("solar.Valid should be true after update")
	}
	if solar.Power != 5000.0 {
		t.Errorf("solar.Power = %f, want %f", solar.Power, 5000.0)
	}
	if solar.LastUpdate.IsZero() {
		t.Error("solar.LastUpdate should not be zero")
	}
	if solar.Message != "" {
		t.Errorf("solar.Message = %q, want empty string", solar.Message)
	}
}

func TestUpdateSolarError(t *testing.T) {
	m := NewManager()

	// First set valid data
	m.UpdateSolar(5000.0)

	// Then set error
	m.UpdateSolarError("No sun")

	solar := m.GetSolarData()
	if solar.Valid {
		t.Error("solar.Valid should be false after error")
	}
	if solar.Message != "No sun" {
		t.Errorf("solar.Message = %q, want %q", solar.Message, "No sun")
	}
}

func TestUpdateFrequency(t *testing.T) {
	m := NewManager()

	// Before any average is computed, API frequency should be invalid
	frequency := m.GetFrequencyData()
	if frequency.Valid {
		t.Error("frequency.Valid should be false before first average")
	}

	// After accumulating and computing the average, API should reflect it
	m.UpdateFrequency(50.01)
	avg := m.GetAndResetAverageFrequency()
	if !avg.Valid {
		t.Fatal("average should be valid after update")
	}

	frequency = m.GetFrequencyData()
	if !frequency.Valid {
		t.Error("frequency.Valid should be true after average computed")
	}
	if frequency.Frequency != 50.01 {
		t.Errorf("frequency.Frequency = %f, want %f", frequency.Frequency, 50.01)
	}
	if frequency.LastUpdate.IsZero() {
		t.Error("frequency.LastUpdate should not be zero")
	}
}

func TestGetAndResetAverageFrequency(t *testing.T) {
	m := NewManager()

	// No samples yet — should return invalid
	avg := m.GetAndResetAverageFrequency()
	if avg.Valid {
		t.Error("average should be invalid when no samples accumulated")
	}

	// Accumulate three samples
	m.UpdateFrequency(50.00)
	m.UpdateFrequency(50.02)
	m.UpdateFrequency(50.04)

	avg = m.GetAndResetAverageFrequency()
	if !avg.Valid {
		t.Fatal("average should be valid after updates")
	}
	want := 50.02
	if diff := avg.Frequency - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("average frequency = %f, want %f", avg.Frequency, want)
	}
	if avg.LastUpdate.IsZero() {
		t.Error("average LastUpdate should not be zero")
	}

	// After reset the accumulator should be empty
	avg = m.GetAndResetAverageFrequency()
	if avg.Valid {
		t.Error("average should be invalid after reset")
	}

	// GetFrequencyData should return the last computed average
	latest := m.GetFrequencyData()
	if !latest.Valid {
		t.Error("GetFrequencyData should still be valid after accumulator reset")
	}
	if latest.Frequency != 50.02 {
		t.Errorf("GetFrequencyData.Frequency = %f, want %f (last average)", latest.Frequency, 50.02)
	}
}

func TestGetAPIResponse(t *testing.T) {
	m := NewManager()
	timestamp := time.Now()

	m.UpdateGrid(100.0, timestamp)
	m.UpdatePrice(0.89)
	m.UpdateSolar(200.0)
	m.UpdateFrequency(50.02)
	m.GetAndResetAverageFrequency() // make frequency visible to API

	response := m.GetAPIResponse()

	if !response.Grid.Valid {
		t.Error("response.Grid.Valid should be true")
	}
	if response.Grid.Power != 100.0 {
		t.Errorf("response.Grid.Power = %f, want %f", response.Grid.Power, 100.0)
	}

	if !response.Price.Valid {
		t.Error("response.Price.Valid should be true")
	}
	if response.Price.Price != 0.89 {
		t.Errorf("response.Price.Price = %f, want %f", response.Price.Price, 0.89)
	}

	if !response.Solar.Valid {
		t.Error("response.Solar.Valid should be true")
	}
	if response.Solar.Power != 200.0 {
		t.Errorf("response.Solar.Power = %f, want %f", response.Solar.Power, 200.0)
	}

	if !response.Frequency.Valid {
		t.Error("response.Frequency.Valid should be true")
	}
	if response.Frequency.Frequency != 50.02 {
		t.Errorf("response.Frequency.Frequency = %f, want %f", response.Frequency.Frequency, 50.02)
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	// Simulate concurrent reads and writes
	for i := 0; i < 100; i++ {
		wg.Add(4)

		go func(val float64) {
			defer wg.Done()
			m.UpdateGrid(val, time.Now())
		}(float64(i))

		go func(val float64) {
			defer wg.Done()
			m.UpdatePrice(val)
		}(float64(i) / 100)

		go func(val float64) {
			defer wg.Done()
			m.UpdateFrequency(50.0 + val/1000)
		}(float64(i))

		go func() {
			defer wg.Done()
			_ = m.GetAPIResponse()
		}()
	}

	wg.Wait()

	// Compute average so frequency becomes visible to API
	m.GetAndResetAverageFrequency()

	// Just verify no panic occurred and state is accessible
	response := m.GetAPIResponse()
	if !response.Grid.Valid {
		t.Error("Grid should be valid after concurrent updates")
	}
	if !response.Price.Valid {
		t.Error("Price should be valid after concurrent updates")
	}
	if !response.Frequency.Valid {
		t.Error("Frequency should be valid after concurrent updates")
	}
}
