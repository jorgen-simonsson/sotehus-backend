package params

import (
	"log/slog"
	"os"
	"testing"

	"github.com/shopspring/decimal"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewStore_SeedsDefaults(t *testing.T) {
	store := newTestStore(t)

	params, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(params) != len(DefaultParams) {
		t.Fatalf("Expected %d default params, got %d", len(DefaultParams), len(params))
	}

	// Verify defaults are present by key
	keys := make(map[string]bool)
	for _, p := range params {
		keys[p.Key] = true
		if p.ID == "" {
			t.Errorf("Parameter %q has empty ID", p.Key)
		}
		if p.Changed.IsZero() {
			t.Errorf("Parameter %q has zero Changed timestamp", p.Key)
		}
	}

	for _, dp := range DefaultParams {
		if !keys[dp.Key] {
			t.Errorf("Default parameter %q not found", dp.Key)
		}
	}
}

func TestNewStore_SeedIdempotent(t *testing.T) {
	// Opening the store twice with the same DB should not duplicate defaults
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Seed again manually
	if err := store.seedDefaults(); err != nil {
		t.Fatalf("seedDefaults failed: %v", err)
	}

	params, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(params) != len(DefaultParams) {
		t.Fatalf("Expected %d params after re-seed, got %d", len(DefaultParams), len(params))
	}
}

func TestGetAll(t *testing.T) {
	store := newTestStore(t)

	params, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(params) < 2 {
		t.Fatalf("Expected at least 2 params, got %d", len(params))
	}

	// Results should be ordered by key
	for i := 1; i < len(params); i++ {
		if params[i-1].Key > params[i].Key {
			t.Errorf("Params not ordered by key: %q > %q", params[i-1].Key, params[i].Key)
		}
	}
}

func TestGetByKey_Found(t *testing.T) {
	store := newTestStore(t)

	p, err := store.GetByKey("DynamicAddPrice")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}

	if p.Key != "DynamicAddPrice" {
		t.Errorf("Key = %q, want %q", p.Key, "DynamicAddPrice")
	}
	if p.Description != "Dynamic addition to price" {
		t.Errorf("Description = %q, want %q", p.Description, "Dynamic addition to price")
	}
	if p.Content != `{"value": 0.0442}` {
		t.Errorf("Content = %q, want %q", p.Content, `{"value": 0.0442}`)
	}
}

func TestGetByKey_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetByKey("NonExistentKey")
	if err == nil {
		t.Fatal("Expected error for non-existent key, got nil")
	}
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestCreate_Success(t *testing.T) {
	store := newTestStore(t)

	req := CreateParamRequest{
		Key:         "TestParam",
		Description: "A test parameter",
		Content:     `{"value": 42}`,
	}

	p, err := store.Create(req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if p.Key != "TestParam" {
		t.Errorf("Key = %q, want %q", p.Key, "TestParam")
	}
	if p.ID == "" {
		t.Error("Expected non-empty ID")
	}
	if p.Changed.IsZero() {
		t.Error("Expected non-zero Changed timestamp")
	}

	// Verify it can be retrieved
	got, err := store.GetByKey("TestParam")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if got.Content != `{"value": 42}` {
		t.Errorf("Content = %q, want %q", got.Content, `{"value": 42}`)
	}
}

func TestCreate_DuplicateKey(t *testing.T) {
	store := newTestStore(t)

	// Try to create a parameter with a key that already exists (seeded default)
	req := CreateParamRequest{
		Key:         "DynamicAddPrice",
		Description: "Duplicate",
		Content:     `{"value": 99}`,
	}

	_, err := store.Create(req)
	if err == nil {
		t.Fatal("Expected error for duplicate key, got nil")
	}
	if err != ErrDuplicateKey {
		t.Errorf("Expected ErrDuplicateKey, got %v", err)
	}
}

func TestUpdate_Success(t *testing.T) {
	store := newTestStore(t)

	// Get the original first
	original, err := store.GetByKey("DynamicAddPrice")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}

	req := UpdateParamRequest{
		Description: "Updated description",
		Content:     `{"value": 0.10}`,
	}

	updated, err := store.Update("DynamicAddPrice", req)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", updated.Description, "Updated description")
	}
	if updated.Content != `{"value": 0.10}` {
		t.Errorf("Content = %q, want %q", updated.Content, `{"value": 0.10}`)
	}
	if updated.ID != original.ID {
		t.Error("ID should not change on update")
	}
	if !updated.Changed.After(original.Changed) && updated.Changed != original.Changed {
		t.Error("Changed timestamp should be updated")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	store := newTestStore(t)

	req := UpdateParamRequest{
		Description: "Does not matter",
		Content:     `{"value": 0}`,
	}

	_, err := store.Update("NonExistentKey", req)
	if err == nil {
		t.Fatal("Expected error for non-existent key, got nil")
	}
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// --- ParseContentValue tests ---

func TestParseContentValue_ValidFloat(t *testing.T) {
	val, err := ParseContentValue(`{"value": 0.2584}`)
	if err != nil {
		t.Fatalf("ParseContentValue failed: %v", err)
	}
	expected := decimal.RequireFromString("0.2584")
	if !val.Equal(expected) {
		t.Errorf("value = %s, want %s", val, expected)
	}
}

func TestParseContentValue_ValidInteger(t *testing.T) {
	val, err := ParseContentValue(`{"value": 25}`)
	if err != nil {
		t.Fatalf("ParseContentValue failed: %v", err)
	}
	if !val.Equal(decimal.NewFromInt(25)) {
		t.Errorf("value = %s, want 25", val)
	}
}

func TestParseContentValue_InvalidJSON(t *testing.T) {
	_, err := ParseContentValue(`not json`)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

func TestParseContentValue_MissingValueKey(t *testing.T) {
	_, err := ParseContentValue(`{"other": 42}`)
	if err == nil {
		t.Fatal("Expected error for missing 'value' key")
	}
}

func TestParseContentValue_NonNumericValue(t *testing.T) {
	_, err := ParseContentValue(`{"value": "hello"}`)
	if err == nil {
		t.Fatal("Expected error for non-numeric value")
	}
}

func TestParseContentValue_Zero(t *testing.T) {
	val, err := ParseContentValue(`{"value": 0}`)
	if err != nil {
		t.Fatalf("ParseContentValue failed: %v", err)
	}
	if !val.Equal(decimal.Zero) {
		t.Errorf("value = %s, want 0", val)
	}
}

func TestParseContentValue_Negative(t *testing.T) {
	val, err := ParseContentValue(`{"value": -1.5}`)
	if err != nil {
		t.Fatalf("ParseContentValue failed: %v", err)
	}
	expected := decimal.RequireFromString("-1.5")
	if !val.Equal(expected) {
		t.Errorf("value = %s, want %s", val, expected)
	}
}
