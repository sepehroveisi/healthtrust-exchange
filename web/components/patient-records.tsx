"use client";

import { motion, useReducedMotion } from "framer-motion";
import { ArrowLeft, Building2, CheckCircle2, FileHeart, ShieldAlert, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { clinicalApi, type ClinicalRecord } from "../lib/api/clinical";
import type { Visit } from "../lib/api/operations-workflow";

type Hospital = "a" | "b";
type Verification = Record<string, boolean>;
type PatientRecordsProps = { patientID: string; sourcePatientID?: string; recordsA: ClinicalRecord[]; recordsB: ClinicalRecord[]; visits: Visit[] };

export function PatientRecords({ patientID, sourcePatientID, recordsA, recordsB, visits }: PatientRecordsProps) {
  const reduce = useReducedMotion();
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [verification, setVerification] = useState<Verification>({});
  const hospitalA = useMemo(() => patientRecords(recordsA, "hospital-a", sourcePatientID ?? patientID), [recordsA, patientID, sourcePatientID]);
  const hospitalB = useMemo(() => patientRecords(recordsB, "hospital-b", patientID), [recordsB, patientID]);
  const all = useMemo(() => [...hospitalA.map(record => ({ record, hospital: "a" as const })), ...hospitalB.map(record => ({ record, hospital: "b" as const }))], [hospitalA, hospitalB]);
  const verificationKey = all.map(value => `${value.hospital}:${value.record.ID}`).sort().join("|");
  const selected = all.find(value => value.record.ID === selectedID);

  useEffect(() => {
    let current = true;
    void Promise.all(all.map(async ({ record, hospital }) => {
      try { return [record.ID, (await clinicalApi.verify(hospital, record.ID)).integrityValid] as const; }
      catch { return [record.ID, false] as const; }
    })).then(values => { if (current) setVerification(Object.fromEntries(values)); });
    return () => { current = false; };
    // The stable identity key prevents periodic polling from re-verifying unchanged records.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [verificationKey]);

  if (selected) return <RecordDetail record={selected.record} hospital={selected.hospital} verified={verification[selected.record.ID]} visit={visits.find(value => value.RecordID === selected.record.ID)} back={() => setSelectedID(null)}/>;

  return <main className="patient-records"><header className="patient-records-heading"><span className="eyebrow">Your care history</span><h1>My Records</h1><p>Your health records across your care network.</p></header>
    <div className="records-network-note"><ShieldCheck/><p><b>Independent hospital records</b><span>Hospital A keeps its original records. Hospital B owns records created during your care there.</span></p></div>
    <HospitalRecords hospital="a" records={hospitalA} verification={verification} open={setSelectedID} reduce={Boolean(reduce)}/>
    <HospitalRecords hospital="b" records={hospitalB} verification={verification} open={setSelectedID} reduce={Boolean(reduce)}/>
  </main>;
}

function HospitalRecords({ hospital, records, verification, open, reduce }: { hospital: Hospital; records: ClinicalRecord[]; verification: Verification; open: (id: string) => void; reduce: boolean }) {
  const name = `Hospital ${hospital.toUpperCase()}`;
  const description = hospital === "a" ? "Original records created at Hospital A." : "Records created during your care at Hospital B.";
  return <section className="hospital-records" data-hospital={hospital} aria-labelledby={`hospital-${hospital}-records`}><header><span className="hospital-records-icon"><Building2/></span><div><h2 id={`hospital-${hospital}-records`}>{name}</h2><p>{description}</p></div>{records.length > 0 && <span className="record-count">{records.length} {records.length === 1 ? "record" : "records"}</span>}</header>
    {hospital === "a" && records.length > 0 && <p className="hospital-ownership"><ShieldCheck/> Created at Hospital A · Original record remains at Hospital A</p>}
    {records.length ? <div className="record-library" role="list">{records.map((record, index) => <motion.article role="listitem" className="patient-record-card" data-record-id={record.ID} key={record.ID} initial={reduce ? false : { opacity: 0, y: 5 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * .04 }}><span className="record-file-icon"><FileHeart/></span><div className="record-card-body"><h3>{record.Diagnosis || "Clinical record"}</h3><p>{name} · {doctorName(record.AuthorDoctorID)} · {formatDate(record.CreatedAt)}</p>{record.EncounterSummary && <span>{record.EncounterSummary}</span>}</div><IntegrityStatus value={verification[record.ID]}/><button className="button secondary" onClick={() => open(record.ID)}>View Record</button></motion.article>)}</div> : <div className="records-empty"><FileHeart/><div><b>No {name} records yet.</b><p>{hospital === "a" ? "Records created at Hospital A will appear here." : "Records created during care at Hospital B will appear here."}</p></div></div>}
  </section>;
}

function RecordDetail({ record, hospital, verified, visit, back }: { record: ClinicalRecord; hospital: Hospital; verified?: boolean; visit?: Visit; back: () => void }) {
  const reduce = useReducedMotion();
  const name = `Hospital ${hospital.toUpperCase()}`;
  const fields = [
    ["Chief complaint", visit?.ChiefComplaint],
    ["Clinical notes", visit?.ClinicalNotes || record.EncounterSummary],
    ["Diagnosis", record.Diagnosis],
    ["Treatment / Plan", visit?.FollowUpPlan],
    ["Prescription / Medication", visit?.Prescription || record.Prescription],
  ].filter((field): field is [string, string] => Boolean(field[1]?.trim()));
  return <motion.main className="patient-record-detail" data-testid="patient-record-detail" initial={reduce ? false : { opacity: 0, x: 5 }} animate={{ opacity: 1, x: 0 }}><button className="record-back" onClick={back}><ArrowLeft/> Back to My Records</button><article><header><span className="eyebrow">Clinical Record</span><h1>{record.Diagnosis || "Clinical record"}</h1><div className="record-detail-meta"><span><Building2/>{name}</span><span>{doctorName(record.AuthorDoctorID)}</span><time dateTime={record.CreatedAt}>{formatDate(record.CreatedAt)}</time></div><IntegrityStatus value={verified} detail/></header>{hospital === "a" && <p className="detail-ownership"><ShieldCheck/><span><b>Created at Hospital A</b>Your original record remains at Hospital A.</span></p>}<dl>{fields.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl></article></motion.main>;
}

function IntegrityStatus({ value, detail = false }: { value?: boolean; detail?: boolean }) {
  if (value === undefined) return <span className={`record-integrity checking${detail ? " detail" : ""}`}><ShieldCheck/>{detail ? "Checking integrity protection" : "Checking integrity"}</span>;
  if (!value) return <span className={`record-integrity failed${detail ? " detail" : ""}`}><ShieldAlert/>{detail ? "Integrity verification needs attention" : "Verification needs attention"}</span>;
  return <span className={`record-integrity verified${detail ? " detail" : ""}`}><CheckCircle2/>{detail ? "Integrity protected · Verified" : "Integrity protected · Verified"}</span>;
}

function patientRecords(records: ClinicalRecord[], organization: string, patientID: string) {
  return [...new Map(records.filter(record => record.PatientID === patientID && record.OrganizationID === organization).map(record => [record.ID, record])).values()].sort((a, b) => new Date(b.CreatedAt).valueOf() - new Date(a.CreatedAt).valueOf());
}

function doctorName(value: string) {
  if (value === "doctor-a") return "Dr. A";
  if (value === "doctor-b") return "Dr. B";
  return "Care clinician";
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", year: "numeric" }).format(new Date(value));
}
