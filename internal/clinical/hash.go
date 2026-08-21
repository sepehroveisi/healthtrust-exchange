package clinical

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

func writeBytes(h hash.Hash, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write(value)
}
func writeString(h hash.Hash, value string) { writeBytes(h, []byte(value)) }
func HashRecord(record ClinicalRecord) [sha256.Size]byte {
	h := sha256.New()
	writeString(h, record.ID)
	writeString(h, record.PatientID)
	writeString(h, record.AuthorDoctorID)
	writeString(h, record.OrganizationID)
	writeString(h, record.EncounterSummary)
	writeString(h, record.Diagnosis)
	writeString(h, record.Prescription)
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(record.CreatedAt.UTC().UnixNano()))
	_, _ = h.Write(timestamp[:])
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}

func HashEvent(values ...string) [sha256.Size]byte {
	h := sha256.New()
	for _, value := range values {
		writeString(h, value)
	}
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}
