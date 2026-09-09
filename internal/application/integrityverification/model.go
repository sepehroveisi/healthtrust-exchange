package integrityverification

import "errors"

type VerificationStatus string

const (
	StatusVerified      VerificationStatus = "VERIFIED"
	StatusFailed        VerificationStatus = "FAILED"
	StatusIndeterminate VerificationStatus = "INDETERMINATE"
)

type CheckType string

const (
	CheckCanonicalCommitment CheckType = "CANONICAL_COMMITMENT"
	CheckLedgerRecord        CheckType = "LEDGER_RECORD"
	CheckLineage             CheckType = "LINEAGE"
	CheckTransaction         CheckType = "TRANSACTION_CONFIRMATION"
	CheckIdentity            CheckType = "IDENTITY"
	CheckResponseState       CheckType = "RESPONSE_STATE"
)

const (
	ClassMismatch            = "MISMATCH"
	ClassUnavailable         = "LEDGER_UNAVAILABLE"
	ClassMissingLedgerRecord = "LEDGER_RECORD_MISSING"
	ClassReceiptUnavailable  = "RECEIPT_UNAVAILABLE"
	ClassReceiptFailed       = "RECEIPT_FAILED"
	ClassUnconfirmed         = "SUBMISSION_NOT_CONFIRMED"
	ClassInvalidLineage      = "INVALID_LINEAGE"
	ClassMissingDomainObject = "MISSING_DOMAIN_OBJECT"
)

var (
	ErrInvalid     = errors.New("integrity verification: invalid identifier")
	ErrNotFound    = errors.New("integrity verification: domain object not found")
	ErrUnavailable = errors.New("integrity verification: required ledger evidence unavailable")
)

type Check struct {
	Type           CheckType          `json:"type"`
	Status         VerificationStatus `json:"status"`
	ObjectID       string             `json:"objectId"`
	Expected       string             `json:"expected,omitempty"`
	Observed       string             `json:"observed,omitempty"`
	Classification string             `json:"classification,omitempty"`
}

type Result struct {
	Status     VerificationStatus `json:"status"`
	ObjectType string             `json:"objectType"`
	ObjectID   string             `json:"objectId"`
	Checks     []Check            `json:"checks"`
}

type OrganizationResult struct {
	OrganizationID string `json:"organizationId"`
	Result         Result `json:"result"`
}

type BundleResult struct {
	Status    VerificationStatus   `json:"status"`
	EventID   string               `json:"eventId"`
	Authority Result               `json:"authority"`
	Responses []OrganizationResult `json:"responses"`
}

func statusOf(checks []Check) VerificationStatus {
	status := StatusVerified
	for _, check := range checks {
		if check.Status == StatusFailed {
			return StatusFailed
		}
		if check.Status == StatusIndeterminate {
			status = StatusIndeterminate
		}
	}
	return status
}

func aggregate(results ...VerificationStatus) VerificationStatus {
	status := StatusVerified
	for _, result := range results {
		if result == StatusFailed {
			return StatusFailed
		}
		if result == StatusIndeterminate {
			status = StatusIndeterminate
		}
	}
	return status
}

func verified(t CheckType, id, expected, observed string) Check {
	return Check{Type: t, Status: StatusVerified, ObjectID: id, Expected: expected, Observed: observed}
}

func failed(t CheckType, id, expected, observed, class string) Check {
	return Check{Type: t, Status: StatusFailed, ObjectID: id, Expected: expected, Observed: observed, Classification: class}
}

func indeterminate(t CheckType, id, class string) Check {
	return Check{Type: t, Status: StatusIndeterminate, ObjectID: id, Classification: class}
}
