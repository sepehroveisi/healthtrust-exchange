package reliability

import (
	"context"
	"crypto/sha256"
	"errors"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency_conflict")
	ErrOperationPending    = errors.New("operation_pending")
)

type IdempotencyResult struct {
	Owner             bool
	ResponseReference string
}
type IdempotencyStore interface {
	BeginIdempotent(context.Context, string, string, string, [sha256.Size]byte) (IdempotencyResult, error)
	CompleteIdempotent(context.Context, string, string, string) error
}

func RequestHash(canonical []byte) [sha256.Size]byte { return sha256.Sum256(canonical) }
