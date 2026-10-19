package domain

import "errors"

// Sentinel errors. Callers wrap them with %w and the HTTP layer maps them with errors.Is.
var (
	ErrValidation        = errors.New("validation failed")
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrNoDriverAvailable = errors.New("no driver available")
	ErrDriverBusy        = errors.New("driver is busy")
	ErrInvalidCoupon     = errors.New("invalid coupon")
	ErrInvalidTransition = errors.New("invalid ride state transition")
)
