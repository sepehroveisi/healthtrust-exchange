"use client";

import { useMemo, useState } from "react";
import type { Patient } from "../lib/api/operations-workflow";
import { clinicalApi, type ClinicalRecord } from "../lib/api/clinical";
import { referralsApi, type Referral } from "../lib/api/referrals";
import { idempotencyKey, newID } from "../lib/api/client";

type Props = { path: string; patients: Patient[]; records: ClinicalRecord[]; referrals: Referral[]; busy: boolean; act: (fn: () => Promise<unknown>) => Promise<void> };

export function DoctorAWorkspace({ path, patients, records, referrals, busy, act }: Props) {
  const [selectedID, setSelectedID] = useState("");
  const [query, setQuery] = useState("");
  const selected = patients.find(value => value.ID === selectedID);
  const visible = useMemo(() => patients.filter(value => `${value.FullName} ${value.ID} ${value.Phone}`.toLowerCase().includes(query.toLowerCase())), [patients, query]);
  const patientRecords = selected ? records.filter(value => value.PatientID === selected.ID) : [];
  const latest = patientRecords.at(-1);
  const referral = latest ? referrals.find(value => value.RecordID === latest.ID) : undefined;
  if (!selected || path === "/app/patients") return <section className="panel section-page"><div className="panel-head"><h2>My Patients</h2></div><label className="doctor-search">Search patients<input value={query} onChange={event => setQuery(event.target.value)} placeholder="Search patients by name, ID or phone..." /></label><div className="patient-directory-list">{visible.map(patient => <article key={patient.ID}><div className="patient-identity"><h3>{patient.FullName}</h3><small>Patient ID {patient.ID}</small></div><div className="patient-context"><span>{records.filter(value => value.PatientID === patient.ID).length} local record(s)</span></div><button className="button secondary" onClick={() => setSelectedID(patient.ID)}>View Patient</button></article>)}</div>{!visible.length && <p>No Hospital A patients match this search.</p>}</section>;
  const createRecord = (form: HTMLFormElement) => { const data = new FormData(form); return clinicalApi.create("a", { ID: newID(), PatientID: selected.ID, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: String(data.get("notes")), Diagnosis: String(data.get("diagnosis")), Prescription: String(data.get("prescription")), CreatedAt: new Date().toISOString() }, idempotencyKey("record")); };
  return <section className="panel section-page"><button className="text-action" onClick={() => setSelectedID("")}>← My Patients</button><div className="panel-head"><h2>{selected.FullName}</h2></div><p>Patient ID: {selected.ID}</p>{latest && <div className="data-row"><b>Medical record saved</b><span>{latest.Diagnosis} · integrity protected</span></div>}<form className="record-form" data-testid="source-visit" onSubmit={event => { event.preventDefault(); void act(() => createRecord(event.currentTarget)); }}><label>Clinical notes<textarea name="notes" defaultValue="Source consultation" /></label><label>Diagnosis<input name="diagnosis" defaultValue="Essential hypertension" /></label><label>Prescription<input name="prescription" defaultValue="Lisinopril 10 mg daily" /></label><button className="button" disabled={busy}>Complete Visit</button></form>{latest && !referral && <button className="button" data-testid="create-referral" onClick={() => void act(() => referralsApi.create({ ID: newID(), PatientID: selected.ID, NetworkPatientID: selected.NetworkPatientID ?? selected.ID, RecordID: latest.ID, SourceOrganizationID: "hospital-a", DestinationOrganizationID: "hospital-b", CreatedByActorID: "doctor-a", Status: "", CreatedAt: new Date().toISOString(), CommitState: "" }, idempotencyKey("referral")))}>Refer Patient to Hospital B</button>}</section>;
}
