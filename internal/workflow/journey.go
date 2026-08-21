package workflow

import (
	"context"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
)

func (s *Service) PatientJourney(ctx context.Context, patient string) (clinical.PatientJourney, error) {
	cross, _ := s.Store.(CrossHospitalStore)
	incoming, _ := cross.ListIncomingReferrals(ctx, patient)
	outgoing, _ := cross.ListOutgoingAccessRequests(ctx, patient)
	referred := len(incoming) > 0
	names := []string{"Checked in", "Dr. B started your visit", "Record access requested", "Your approval is required", "Previous record securely available", "Dr. B securely viewed the record", "Visit completed"}
	if referred {
		names = append([]string{"Referral created", "Hospital B received your referral"}, names...)
	}
	j := clinical.PatientJourney{PatientID: patient, CurrentStage: names[0], Steps: make([]clinical.JourneyStep, len(names))}
	for i, name := range names {
		j.Steps[i] = clinical.JourneyStep{Name: name, Status: "Pending"}
	}
	offset := 0
	if referred {
		j.Steps[0].Status = "Complete"
		j.Steps[1].Status = "Complete"
		offset = 2
		j.CurrentStage = names[1]
	}
	recordIDs := map[string]struct{}{}
	if len(outgoing) > 0 {
		q := outgoing[len(outgoing)-1].Request
		j.Steps[offset+2].Status = "Complete"
		j.CurrentStage = names[offset+2]
		if q.RecordID != "" {
			recordIDs[q.RecordID] = struct{}{}
		}
		if q.Status == clinical.AccessGranted || q.Status == clinical.AccessRevoked {
			j.Steps[offset+3].Status = "Complete"
			j.Steps[offset+4].Status = "Complete"
			j.CurrentStage = names[offset+4]
		}
	}
	if ops, ok := s.Store.(OperationalStore); ok {
		visits, _ := ops.ListVisits(ctx, s.OrganizationID, "")
		for _, visit := range visits {
			if visit.PatientID != patient {
				continue
			}
			j.Steps[offset].Status = "Complete"
			j.CurrentStage = names[offset]
			if visit.Status == clinical.VisitInVisit || visit.Status == clinical.VisitCompleted {
				j.Steps[offset+1].Status = "Complete"
				j.CurrentStage = names[offset+1]
			}
			if visit.Status == clinical.VisitCompleted {
				j.Steps[offset+6].Status = "Complete"
				j.CurrentStage = names[offset+6]
			}
		}
	}
	if n, ok := s.Node.(interface{ Blocks() []blockchain.Block }); ok {
		for _, block := range n.Blocks() {
			for _, tx := range block.Transactions {
				if tx.Type == blockchain.RecordAccessed {
					if _, ok := recordIDs[tx.ResourceID]; ok {
						j.Steps[offset+5].Status = "Complete"
						j.CurrentStage = names[offset+5]
					}
				}
			}
		}
	}
	return j, nil
}
