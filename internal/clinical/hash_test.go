package clinical

import (
	"testing"
	"time"
)

func TestHashRecordDeterministicAndSensitive(t *testing.T) {
	base := ClinicalRecord{ID: "r", PatientID: "p", AuthorDoctorID: "d", OrganizationID: "o", EncounterSummary: "e", Diagnosis: "x", Prescription: "y", CreatedAt: time.Unix(1, 2)}
	if HashRecord(base) != HashRecord(base) {
		t.Fatal("hash not deterministic")
	}
	tests := []struct {
		name   string
		mutate func(*ClinicalRecord)
	}{{"diagnosis", func(r *ClinicalRecord) { r.Diagnosis = "changed" }}, {"prescription", func(r *ClinicalRecord) { r.Prescription = "changed" }}, {"author", func(r *ClinicalRecord) { r.AuthorDoctorID = "changed" }}, {"patient", func(r *ClinicalRecord) { r.PatientID = "changed" }}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			tt.mutate(&changed)
			if HashRecord(base) == HashRecord(changed) {
				t.Fatal("modified record hash unchanged")
			}
		})
	}
}
