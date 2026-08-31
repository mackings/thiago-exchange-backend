package domain

import "errors"

var (
	ErrNotFound              = errors.New("resource not found")
	ErrAlreadyExists         = errors.New("resource already exists")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrForbidden             = errors.New("forbidden")
	ErrInvalidInput          = errors.New("invalid input")
	ErrInsufficientBalance   = errors.New("insufficient balance")
	ErrInvalidOrderState     = errors.New("invalid order state transition")
	ErrAdUnavailable         = errors.New("ad is not available")
	ErrDepositNotFound       = errors.New("no matching confirmed deposit found yet")
	ErrAddressNotWhitelisted = errors.New("payout address is not whitelisted on Bybit yet")
	ErrDepositAddressNotSet  = errors.New("deposit address has not been set for this asset yet")
)
