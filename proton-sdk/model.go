// Package protonsdk defines the provider-neutral model boundary used by Protonman agents.
package protonsdk

import "errors"

var (
	ErrInvalidRequest   = errors.New("invalid model request")
	ErrInvalidEvent     = errors.New("invalid model event")
	ErrIncompleteStream = errors.New("incomplete model stream")
)
