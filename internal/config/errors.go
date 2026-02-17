package config

import "errors"

// ErrNotFound is returned when a requested resource does not exist in the store.
var ErrNotFound = errors.New("not found")
