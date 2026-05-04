package rss

import "errors"

var (
	// ErrSourceNotFound is returned when a requested source is not registered.
	ErrSourceNotFound = errors.New("rss: source not found")

	// ErrInvalidGUID is returned when an item has an invalid GUID.
	ErrInvalidGUID = errors.New("rss: invalid GUID")

	// ErrInvalidSource is returned when a source ID is invalid.
	ErrInvalidSource = errors.New("rss: invalid source")
)
