package params

import "errors"

var (
	// ErrNotFound is returned when a parameter is not found
	ErrNotFound = errors.New("parameter not found")

	// ErrDuplicateKey is returned when attempting to create a parameter with an existing key
	ErrDuplicateKey = errors.New("parameter with this key already exists")
)
