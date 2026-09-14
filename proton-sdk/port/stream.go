package port

import (
	"context"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

type Stream interface {
	Next(ctx context.Context) (domain.Event, error)
	Close() error
}
