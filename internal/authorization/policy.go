package authorization

import (
	"errors"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

var ErrUnauthorizedTransaction = errors.New("unauthorized transaction")

// Policy decides whether a trusted actor role may submit a transaction type.
// A string type keeps authorization independent from blockchain data structures.
type Policy interface {
	CanSubmit(actor identity.Actor, transactionType string) error
}
