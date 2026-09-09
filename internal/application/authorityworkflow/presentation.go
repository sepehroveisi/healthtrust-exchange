package authorityworkflow

import "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"

// ActionPresentation is bounded display metadata for the synthetic RC2 demo.
// It deliberately contains no persisted policy, decision, or action detail.
type ActionPresentation struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	DemoOnly bool   `json:"demoOnly"`
}

type demoActionIdentity struct {
	organizationID authorityledger.OrganizationID
	actionID       string
}

var demoActionPresentations = map[demoActionIdentity]ActionPresentation{
	{"HOSPITAL-A", "ACT-P8-HOSP"}:         {Code: "SCHEDULING_DISABLED", Label: "Scheduling disabled", DemoOnly: true},
	{"PAYER-B", "ACT-P8-PAYER"}:           {Code: "ENROLLMENT_HELD", Label: "Enrollment/reimbursement held", DemoOnly: true},
	{"STAFFING-AGENCY-C", "ACT-P8-STAFF"}: {Code: "ASSIGNMENT_ENDED", Label: "Assignment ended", DemoOnly: true},
}

func actionPresentation(response authorityledger.ResponseVersion) *ActionPresentation {
	if response.State != "ACTION_COMPLETED" || response.ActionID == nil {
		return nil
	}
	value, ok := demoActionPresentations[demoActionIdentity{response.OrganizationID, *response.ActionID}]
	if !ok {
		return nil
	}
	return &value
}
