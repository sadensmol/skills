package domain

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrNotOpen       = errors.New("ask is not open")
	// ErrAskPending means an ask is still open when a wait reached its limit.
	ErrAskPending = errors.New("ask is still open")
	// ErrAskDismissed means the owner closed the dialog without an answer.
	ErrAskDismissed = errors.New("ask was dismissed")
)
