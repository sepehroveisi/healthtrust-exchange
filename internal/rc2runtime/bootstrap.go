package rc2runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/authorityworkflow"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/tamperdemo"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/jackc/pgx/v5"
)

const (
	FixtureAuthorityID = tamperdemo.FixtureAuthorityID
	FixtureSubjectID   = tamperdemo.FixtureSubjectID
	FixtureEventID     = tamperdemo.FixtureEventID
	FixtureSeriesID    = "SERIES-PHASE8"
	FixtureHospitalID  = "HOSPITAL-A"
	FixturePayerID     = "PAYER-B"
	FixtureStaffingID  = "STAFFING-AGENCY-C"

	fixtureSource         = "synthetic private Phase 8 authority source"
	mutatedFixtureSource  = "phase8-demo-mutated-authority-source"
	mutatedHospitalPolicy = "phase8-demo-mutated-hospital-policy"
)

var fixtureTime = time.Date(2026, 7, 1, 0, 0, 0, 123, time.UTC)

type bootstrapStore interface {
	GetAuthority(context.Context, authorityledger.AuthorityID) (authorityledger.Authority, error)
	CreateAuthority(context.Context, authorityledger.Authority) error
	GetSubject(context.Context, authorityledger.SubjectID) (authorityledger.ProfessionalSubject, error)
	CreateSubject(context.Context, authorityledger.ProfessionalSubject) error
	GetOrganization(context.Context, authorityledger.OrganizationID) (authorityledger.Organization, error)
	CreateOrganization(context.Context, authorityledger.Organization) error
	GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error)
	GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error)
	ListResponseStreamsForEvent(context.Context, authorityledger.AssertionID) ([]authorityledger.ResponseStream, error)
	GetResponseStreamForEvent(context.Context, authorityledger.AssertionID, authorityledger.OrganizationID) (authorityledger.ResponseStream, error)
	ResponseHistory(context.Context, authorityledger.ResponseID) ([]authorityledger.ResponseVersion, error)
	GetPolicy(context.Context, string) (authorityledger.Policy, error)
	GetDecision(context.Context, string) (authorityledger.Decision, error)
	GetAction(context.Context, string) (authorityledger.Action, error)
	GetSubmissionByDomain(context.Context, string, string) (authorityledger.Submission, error)
}

type bootstrapWorkflow interface {
	RegisterAuthorityEvent(context.Context, authorityworkflow.AuthorityEventInput) (authorityworkflow.EventView, error)
	StartOrganizationResponse(context.Context, authorityworkflow.StartResponseInput) (authorityworkflow.ResponseVersionView, error)
	AdvanceOrganizationResponse(context.Context, authorityworkflow.AdvanceResponseInput) (authorityworkflow.ResponseVersionView, error)
}

type bundleVerifier interface {
	VerifyEventBundle(context.Context, string) (integrityverification.BundleResult, error)
}

type fixtureFlow struct {
	organization string
	short        string
	response     string
}

var fixtureFlows = []fixtureFlow{
	{FixtureHospitalID, "HOSP", "P8-HOSP"},
	{FixturePayerID, "PAYER", "P8-PAYER"},
	{FixtureStaffingID, "STAFF", "P8-STAFF"},
}

func Bootstrap(ctx context.Context, store bootstrapStore, workflow bootstrapWorkflow, verifier bundleVerifier, signerKeys []string) error {
	if len(signerKeys) != 4 {
		return errors.New("four application signer keys are required")
	}
	addresses := make([]authorityledger.Address, 4)
	for index, key := range signerKeys {
		privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(key, "0x"))
		if err != nil {
			return fmt.Errorf("invalid application signer %d", index)
		}
		copy(addresses[index][:], crypto.PubkeyToAddress(privateKey.PublicKey).Bytes())
	}
	if err := ensurePrerequisites(ctx, store, addresses); err != nil {
		return err
	}
	assertion, err := store.GetAssertion(ctx, FixtureEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, seriesErr := store.GetEventSeries(ctx, FixtureSeriesID); !errors.Is(seriesErr, pgx.ErrNoRows) {
			if seriesErr == nil {
				return errors.New("fixture series exists without its assertion")
			}
			return seriesErr
		}
		if err = seedFixture(ctx, workflow); err != nil {
			return err
		}
		assertion, err = store.GetAssertion(ctx, FixtureEventID)
		if err != nil {
			return err
		}
		return validateFixture(ctx, store, verifier, assertion)
	}
	if err != nil {
		return err
	}
	return validateFixture(ctx, store, verifier, assertion)
}

func ensurePrerequisites(ctx context.Context, store bootstrapStore, addresses []authorityledger.Address) error {
	authority := authorityledger.Authority{ID: FixtureAuthorityID, Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: addresses[0]}
	if current, err := store.GetAuthority(ctx, authority.ID); errors.Is(err, pgx.ErrNoRows) {
		if err = store.CreateAuthority(ctx, authority); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if current != authority {
		return errors.New("fixture authority conflicts with expected configuration")
	}
	subject := authorityledger.ProfessionalSubject{ID: FixtureSubjectID, DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}
	if current, err := store.GetSubject(ctx, subject.ID); errors.Is(err, pgx.ErrNoRows) {
		if err = store.CreateSubject(ctx, subject); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if current.ID != subject.ID || current.DisplayName != subject.DisplayName || !validSubjectDetails(current.Details) {
		return errors.New("fixture subject conflicts with expected configuration")
	}
	organizations := []authorityledger.Organization{
		{ID: FixtureHospitalID, Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: addresses[1]},
		{ID: FixturePayerID, Type: "PAYER", DisplayName: "Payer B", Active: true, Signer: addresses[2]},
		{ID: FixtureStaffingID, Type: "STAFFING_AGENCY", DisplayName: "Staffing Agency C", Active: true, Signer: addresses[3]},
	}
	for _, organization := range organizations {
		current, err := store.GetOrganization(ctx, organization.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			if err = store.CreateOrganization(ctx, organization); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if current != organization {
			return fmt.Errorf("fixture organization %s conflicts with expected configuration", organization.ID)
		}
	}
	return nil
}

func validSubjectDetails(raw []byte) bool {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || len(value) != 1 {
		return false
	}
	synthetic, ok := value["synthetic"].(bool)
	return ok && synthetic
}

func seedFixture(ctx context.Context, workflow bootstrapWorkflow) error {
	source := []byte(fixtureSource)
	event, err := workflow.RegisterAuthorityEvent(ctx, authorityworkflow.AuthorityEventInput{
		EventID: FixtureEventID, SeriesID: FixtureSeriesID, AuthorityID: FixtureAuthorityID,
		SubjectID: FixtureSubjectID, OperationID: "OP-P8-EVENT", EffectiveTime: fixtureTime,
		SourceDocumentHash: canonical.Hash(source), SourceDocument: source,
	})
	if err != nil || event.Ledger.State != "CONFIRMED" {
		return fmt.Errorf("create fixture authority event: state=%s: %w", event.Ledger.State, err)
	}
	for index, flow := range fixtureFlows {
		receiptTime := fixtureTime.Add(time.Duration(index+1) * time.Hour)
		start, startErr := workflow.StartOrganizationResponse(ctx, authorityworkflow.StartResponseInput{
			ActorOrganizationID: flow.organization, OrganizationID: flow.organization, EventID: FixtureEventID,
			ResponseID: flow.response, VersionID: flow.response + "-V1", OperationID: "OP-P8-" + flow.short + "-V1", ReceiptTime: receiptTime,
		})
		if startErr != nil || start.Ledger.State != "CONFIRMED" {
			return fmt.Errorf("start %s fixture response: state=%s: %w", flow.organization, start.Ledger.State, startErr)
		}
		for stateIndex, state := range []string{"UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"} {
			version := stateIndex + 2
			suffix := fmt.Sprintf("%d", version)
			advanced, advanceErr := workflow.AdvanceOrganizationResponse(ctx, authorityworkflow.AdvanceResponseInput{
				ActorOrganizationID: flow.organization, OrganizationID: flow.organization, EventID: FixtureEventID,
				VersionID: flow.response + "-V" + suffix, OperationID: "OP-P8-" + flow.short + "-V" + suffix,
				TargetState: state, OccurredAt: receiptTime.Add(time.Duration(version) * time.Hour),
				PolicyID: "POL-P8-" + flow.short, PolicyVersionID: "POL-P8-" + flow.short + "-V1", PolicyText: "private " + flow.short + " policy",
				DecisionID: "DEC-P8-" + flow.short, DecisionCode: "HOLD-" + flow.short, DecisionExplanation: "private " + flow.short + " decision", ReviewerReference: "REVIEW-" + flow.short,
				ActionID: "ACT-P8-" + flow.short, ActionCode: "COMPLETE-" + flow.short, ActionExplanation: "private " + flow.short + " action", OperatorReference: "OPERATOR-" + flow.short,
				SupportingEvidence: []byte("private " + flow.short + " support"),
			})
			if advanceErr != nil || advanced.Ledger.State != "CONFIRMED" {
				return fmt.Errorf("advance %s to %s: state=%s: %w", flow.organization, state, advanced.Ledger.State, advanceErr)
			}
		}
	}
	return nil
}

func validateFixture(ctx context.Context, store bootstrapStore, verifier bundleVerifier, assertion authorityledger.AuthorityAssertion) error {
	series, err := store.GetEventSeries(ctx, FixtureSeriesID)
	if err != nil {
		return fmt.Errorf("fixture series: %w", err)
	}
	if assertion.ID != FixtureEventID || assertion.SeriesID != FixtureSeriesID || assertion.EventType != "EXCLUSION" || assertion.Kind != "ORIGINAL" || assertion.Effect != "EXCLUSION_ACTIVE" || assertion.EffectiveUnixNano != fixtureTime.UnixNano() || series.ID != FixtureSeriesID || series.AuthorityID != FixtureAuthorityID || series.SubjectID != FixtureSubjectID {
		return errors.New("fixture authority event conflicts with expected metadata")
	}
	if string(assertion.SourceDocument) != fixtureSource && string(assertion.SourceDocument) != mutatedFixtureSource {
		return errors.New("fixture authority source is neither clean nor the controlled demo mutation")
	}
	if err = confirmed(ctx, store, submission.OperationAuthorityAssertion, FixtureEventID); err != nil {
		return err
	}
	streams, err := store.ListResponseStreamsForEvent(ctx, FixtureEventID)
	if err != nil || len(streams) != len(fixtureFlows) {
		return fmt.Errorf("fixture must contain exactly three response streams: %w", err)
	}
	for _, flow := range fixtureFlows {
		if err = validateFlow(ctx, store, flow); err != nil {
			return err
		}
	}
	bundle, verifyErr := verifier.VerifyEventBundle(ctx, FixtureEventID)
	if verifyErr != nil {
		return fmt.Errorf("fixture verification unavailable: %w", verifyErr)
	}
	if bundle.Status != integrityverification.StatusVerified && !expectedControlledFailure(bundle) {
		return errors.New("fixture is in an unexpected integrity state")
	}
	return nil
}

func validateFlow(ctx context.Context, store bootstrapStore, flow fixtureFlow) error {
	stream, err := store.GetResponseStreamForEvent(ctx, FixtureEventID, authorityledger.OrganizationID(flow.organization))
	if err != nil || string(stream.ID) != flow.response || stream.EventID != FixtureEventID || string(stream.OrganizationID) != flow.organization {
		return fmt.Errorf("fixture response stream %s conflicts: %w", flow.organization, err)
	}
	history, err := store.ResponseHistory(ctx, stream.ID)
	if err != nil || len(history) != 4 {
		return fmt.Errorf("fixture response history %s must contain four versions: %w", flow.organization, err)
	}
	states := []string{"RECEIVED", "UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"}
	for index, version := range history {
		expectedID := flow.response + fmt.Sprintf("-V%d", index+1)
		if string(version.ID) != expectedID || version.State != states[index] || string(version.ResponseID) != flow.response || string(version.EventID) != FixtureEventID || string(version.OrganizationID) != flow.organization {
			return fmt.Errorf("fixture response %s version %d conflicts", flow.organization, index+1)
		}
		if index == 0 && version.PreviousID != nil || index > 0 && (version.PreviousID == nil || string(*version.PreviousID) != flow.response+fmt.Sprintf("-V%d", index)) {
			return fmt.Errorf("fixture response %s lineage conflicts", flow.organization)
		}
		if err = confirmed(ctx, store, submission.OperationResponseVersion, expectedID); err != nil {
			return err
		}
	}
	latest := history[3]
	if latest.PolicyVersionID == nil || latest.DecisionID == nil || latest.ActionID == nil {
		return fmt.Errorf("fixture response %s is incomplete", flow.organization)
	}
	policy, err := store.GetPolicy(ctx, *latest.PolicyVersionID)
	if err != nil || policy.VersionID != "POL-P8-"+flow.short+"-V1" || string(policy.OrganizationID) != flow.organization {
		return fmt.Errorf("fixture policy %s conflicts: %w", flow.organization, err)
	}
	expectedPolicy := "private " + flow.short + " policy"
	if policy.Text != expectedPolicy && !(flow.organization == FixtureHospitalID && policy.Text == mutatedHospitalPolicy) {
		return fmt.Errorf("fixture policy %s has an unexpected value", flow.organization)
	}
	decision, err := store.GetDecision(ctx, *latest.DecisionID)
	if err != nil || decision.ID != "DEC-P8-"+flow.short || string(decision.OrganizationID) != flow.organization || string(decision.EventID) != FixtureEventID {
		return fmt.Errorf("fixture decision %s conflicts: %w", flow.organization, err)
	}
	action, err := store.GetAction(ctx, *latest.ActionID)
	if err != nil || action.ID != "ACT-P8-"+flow.short || string(action.OrganizationID) != flow.organization || string(action.EventID) != FixtureEventID {
		return fmt.Errorf("fixture action %s conflicts: %w", flow.organization, err)
	}
	return nil
}

func confirmed(ctx context.Context, store bootstrapStore, kind, domainID string) error {
	value, err := store.GetSubmissionByDomain(ctx, kind, domainID)
	if err != nil {
		return fmt.Errorf("fixture submission %s: %w", domainID, err)
	}
	if value.State != "CONFIRMED" || len(value.TransactionHash) != 32 || value.BlockNumber == nil || len(value.BlockHash) != 32 {
		return fmt.Errorf("fixture submission %s is not confirmed", domainID)
	}
	return nil
}

func expectedControlledFailure(bundle integrityverification.BundleResult) bool {
	authorityFailed := bundle.Authority.Status == integrityverification.StatusFailed
	hospitalFailed := false
	for _, response := range bundle.Responses {
		switch response.OrganizationID {
		case FixtureHospitalID:
			hospitalFailed = response.Result.Status == integrityverification.StatusFailed
		case FixturePayerID, FixtureStaffingID:
			if response.Result.Status != integrityverification.StatusVerified {
				return false
			}
		default:
			return false
		}
	}
	return (authorityFailed && !hospitalFailed) || (!authorityFailed && hospitalFailed)
}
