package authorityworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

func TestDemoActionPresentation(t *testing.T) {
	tests := []struct {
		organization, action, code, label string
	}{
		{"HOSPITAL-A", "ACT-P8-HOSP", "SCHEDULING_DISABLED", "Scheduling disabled"},
		{"PAYER-B", "ACT-P8-PAYER", "ENROLLMENT_HELD", "Enrollment/reimbursement held"},
		{"STAFFING-AGENCY-C", "ACT-P8-STAFF", "ASSIGNMENT_ENDED", "Assignment ended"},
	}
	for _, test := range tests {
		t.Run(test.organization, func(t *testing.T) {
			actionID := test.action
			value := actionPresentation(authorityledger.ResponseVersion{
				OrganizationID: authorityledger.OrganizationID(test.organization),
				State:          "ACTION_COMPLETED", ActionID: &actionID,
			})
			if value == nil || value.Code != test.code || value.Label != test.label || !value.DemoOnly {
				t.Fatalf("unexpected presentation: %+v", value)
			}
		})
	}
}

func TestDemoActionPresentationFailsClosed(t *testing.T) {
	actionID := "ACT-P8-HOSP"
	for _, response := range []authorityledger.ResponseVersion{
		{OrganizationID: "PAYER-B", State: "ACTION_COMPLETED", ActionID: &actionID},
		{OrganizationID: "HOSPITAL-A", State: "DECIDED", ActionID: &actionID},
		{OrganizationID: "HOSPITAL-A", State: "ACTION_COMPLETED"},
	} {
		if value := actionPresentation(response); value != nil {
			t.Fatalf("unexpected presentation for non-allowlisted identity: %+v", value)
		}
	}
}

func TestResponseViewPresentationSerializationIsBounded(t *testing.T) {
	encoded, err := json.Marshal(ResponseVersionView{
		ID: "P8-HOSP-V4", State: "ACTION_COMPLETED",
		ActionPresentation: &ActionPresentation{Code: "SCHEDULING_DISABLED", Label: "Scheduling disabled", DemoOnly: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, expected := range []string{`"actionPresentation"`, `"code":"SCHEDULING_DISABLED"`, `"label":"Scheduling disabled"`, `"demoOnly":true`} {
		if !strings.Contains(jsonText, expected) {
			t.Fatalf("missing safe presentation field %s in %s", expected, jsonText)
		}
	}
	for _, forbidden := range []string{"PolicyText", "DecisionExplanation", "ActionExplanation", "ReviewerReference", "OperatorReference", "SupportingEvidence", "SourceDocument", "private HOSP action"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("sensitive field %s serialized in %s", forbidden, jsonText)
		}
	}
}
