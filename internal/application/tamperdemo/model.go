package tamperdemo

import (
	"errors"
	"os"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
)

const (
	EnvironmentVariable = "HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED"
	DemoWarning         = "Controlled synthetic-data mutation for integrity demonstration only."

	FixtureEventID      = "EVENT-PHASE8"
	FixtureAuthorityID  = "HHS-OIG-DEMO"
	FixtureSubjectID    = "PRV-7F31A"
	FixtureOrganization = "HOSPITAL-A"
)

type Target string

const (
	TargetAuthorityEvent  Target = "AUTHORITY_EVENT"
	TargetHospitalAPolicy Target = "HOSPITAL_A_POLICY"
)

var (
	ErrDemoTamperDisabled       = errors.New("controlled tamper demo: disabled")
	ErrUnsupportedTarget        = errors.New("controlled tamper demo: unsupported target")
	ErrSyntheticFixtureMissing  = errors.New("controlled tamper demo: synthetic fixture missing")
	ErrSyntheticFixtureMismatch = errors.New("controlled tamper demo: synthetic fixture mismatch")
	ErrPrecondition             = errors.New("controlled tamper demo: target is not verified")
	ErrVerificationUnavailable  = errors.New("controlled tamper demo: verification unavailable")
	ErrMutationFailed           = errors.New("controlled tamper demo: PostgreSQL mutation failed")
	ErrPostcondition            = errors.New("controlled tamper demo: mismatch was not established")
)

func EnabledFromEnvironment() bool {
	return os.Getenv(EnvironmentVariable) == "true"
}

type Snapshot struct {
	Status           integrityverification.VerificationStatus `json:"status"`
	LocalCommitment  string                                   `json:"localCommitment"`
	LedgerCommitment string                                   `json:"ledgerCommitment"`
	Verification     integrityverification.Result             `json:"verification"`
}

type Result struct {
	DemoOnly        bool     `json:"demoOnly"`
	Warning         string   `json:"warning"`
	Target          Target   `json:"target"`
	EventID         string   `json:"eventId"`
	OrganizationID  string   `json:"organizationId,omitempty"`
	MutationType    string   `json:"mutationType"`
	ChangedField    string   `json:"changedField"`
	Before          Snapshot `json:"before"`
	After           Snapshot `json:"after"`
	LedgerUnchanged bool     `json:"ledgerUnchanged"`
}
