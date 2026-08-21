package authorization

import (
	"fmt"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

type HealthcarePolicy struct{}

var allowedTransactions = map[identity.Role]map[string]struct{}{
	identity.RoleDoctor: {
		"RecordCommitted": {}, "RecordAccessed": {}, "ReferralCreated": {},
	},
	identity.RolePatient: {
		"ConsentGranted": {}, "ConsentRevoked": {},
	},
	identity.RoleStaff: {
		"ReferralCreated": {},
	},
}

func (HealthcarePolicy) CanSubmit(actor identity.Actor, transactionType string) error {
	if _, allowed := allowedTransactions[actor.Role][transactionType]; !allowed {
		return fmt.Errorf("%w: role %s cannot submit %s", ErrUnauthorizedTransaction, actor.Role, transactionType)
	}
	return nil
}
