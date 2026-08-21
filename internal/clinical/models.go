package clinical

import "time"

type CommitState string

const (
	CommitPending   CommitState = "Pending"
	CommitCommitted CommitState = "Committed"
	CommitFailed    CommitState = "Failed"
)

type ClinicalRecord struct {
	ID, PatientID, AuthorDoctorID, OrganizationID, EncounterSummary, Diagnosis, Prescription string
	CreatedAt, UpdatedAt                                                                     time.Time
	CommitState                                                                              CommitState
	RecordHash                                                                               []byte
}
type ReferralStatus string

const (
	ReferralCreated         ReferralStatus = "Created"
	ReferralDelivered       ReferralStatus = "Delivered"
	ReferralReceived        ReferralStatus = "Received"
	ReferralAccessRequested ReferralStatus = "AccessRequested"
	ReferralCompleted       ReferralStatus = "Completed"
	ReferralPending         ReferralStatus = "Pending" // retained for persisted pre-repair data
	ReferralAccepted        ReferralStatus = "Accepted"
	ReferralCancelled       ReferralStatus = "Cancelled"
)

type Referral struct {
	ID, PatientID, NetworkPatientID, RecordID, SourceOrganizationID, DestinationOrganizationID, CreatedByActorID string
	Status                                                                                                       ReferralStatus
	CreatedAt                                                                                                    time.Time
	CommitState                                                                                                  CommitState
}
type AccessStatus string

const (
	AccessPending AccessStatus = "Pending"
	AccessGranted AccessStatus = "Granted"
	AccessDenied  AccessStatus = "Denied"
	AccessRevoked AccessStatus = "Revoked"
)

type AccessRequest struct {
	ID, ReferralID, PatientID, NetworkPatientID, RecordID, DiscoveryScopeID  string
	RequesterActorID, RequesterOrganizationID, SourceOrganizationID, Purpose string
	Status                                                                   AccessStatus
	CreatedAt, UpdatedAt                                                     time.Time
}
type ConsentStatus string

const (
	ConsentPending ConsentStatus = "Pending"
	ConsentActive  ConsentStatus = "Active"
	// ConsentRevocationPending is fail-closed: source-side authorization only
	// accepts ConsentActive, while blockchain evidence/finalization is retried.
	ConsentRevocationPending ConsentStatus = "RevocationPending"
	ConsentRevoked           ConsentStatus = "Revoked"
	ConsentFailed            ConsentStatus = "Failed"
)

type Consent struct {
	ID, AccessRequestID, ReferralID, PatientID, RecordID, GrantedToActorID, GrantedToOrganizationID string
	Status                                                                                          ConsentStatus
	GrantedAt                                                                                       time.Time
	RevokedAt, ExpiresAt                                                                            *time.Time
}

// IncomingReferral is Hospital B's non-clinical projection of a referral owned by Hospital A.
type IncomingReferral struct {
	Referral      Referral
	TransactionID string
	ReceivedAt    time.Time
}

// OutgoingAccessRequest is Hospital B's local record of a request forwarded to Hospital A.
type OutgoingAccessRequest struct {
	Request                                         AccessRequest
	SourceOrganizationID, DestinationOrganizationID string
}

type JourneyStep struct {
	Name, Status string
	OccurredAt   *time.Time
}
type PatientJourney struct {
	PatientID, CurrentStage string
	Steps                   []JourneyStep
}
type Patient struct {
	ID, OrganizationID, NetworkPatientID, FullName, DateOfBirth, Gender, Phone, Address, EmergencyContact string
	CreatedAt                                                                                             time.Time
}

// RecordDiscovery is deliberately non-clinical. Scope IDs are opaque, short-lived
// request targets; they are not record IDs and cannot be used to retrieve content.
type RecordDiscovery struct {
	Hospital, OrganizationID string
	RecordsAvailable         bool
	RecordCount              int
	Scopes                   []DiscoveredRecordScope
}

type DiscoveredRecordScope struct {
	ID, Label string
}
type VisitStatus string

const (
	VisitWaiting   VisitStatus = "Waiting"
	VisitInVisit   VisitStatus = "InVisit"
	VisitCompleted VisitStatus = "Completed"
)

type VisitSource string

const (
	VisitNormal   VisitSource = "Normal"
	VisitReferral VisitSource = "Referral"
)

type Visit struct {
	ID, PatientID, OrganizationID, DoctorID, Room, Reason, ReferralID              string
	Source                                                                         VisitSource
	Status                                                                         VisitStatus
	CheckedInAt                                                                    time.Time
	StartedAt, CompletedAt                                                         *time.Time
	ChiefComplaint, ClinicalNotes, Diagnosis, Prescription, FollowUpPlan, RecordID string
}
