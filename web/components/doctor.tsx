"use client";

import { motion, useReducedMotion } from "framer-motion";
import { useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, FileHeart, LockKeyhole, ShieldCheck, X } from "lucide-react";
import { careApi, type Patient, type Visit } from "../lib/api/operations-workflow";
import { clinicalApi, type ClinicalRecord } from "../lib/api/clinical";
import { accessApi, type AccessRequest, type RecordDiscovery } from "../lib/api/access";
import type { Referral } from "../lib/api/referrals";
import { newID } from "../lib/api/client";

type RetrievedExternalRecord = { record: ClinicalRecord; patientID: string; requestID: string; recordID: string; sourceOrganizationID: string; discoveryScopeID: string };
type DoctorProps = { path: string; patients: Patient[]; visits: Visit[]; records: ClinicalRecord[]; referrals: Referral[]; requests: AccessRequest[]; selected: Visit | null; select: (visit: Visit | null) => void; busy: boolean; act: (fn: () => Promise<unknown>) => Promise<void> };

const sameDay = (value: string, now = new Date()) => { const date = new Date(value); return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate(); };
const byArrival = (a: Visit, b: Visit) => new Date(a.CheckedInAt).valueOf() - new Date(b.CheckedInAt).valueOf();
const age = (dob: string) => { const birth = new Date(dob), now = new Date(); let value = now.getFullYear() - birth.getFullYear(); if (now.getMonth() < birth.getMonth() || (now.getMonth() === birth.getMonth() && now.getDate() < birth.getDate())) value--; return Math.max(0, value); };
const waitingFor = (value: string) => { const minutes = Math.max(0, Math.floor((Date.now() - new Date(value).valueOf()) / 60000)); return minutes < 1 ? "Waiting less than a minute" : `Waiting ${minutes} min`; };
const patientFor = (patients: Patient[], visit: Visit) => patients.find(value => value.ID === visit.PatientID);

export function DoctorWorkspace(props: DoctorProps) {
  const mine = useMemo(() => props.visits.filter(value => value.DoctorID === "doctor-b").sort(byArrival), [props.visits]);
  const operational = useMemo(() => mine.filter(value => sameDay(value.CheckedInAt)), [mine]);
  const [directoryPatient, setDirectoryPatient] = useState<string | null>(null);
  const [retrieved, setRetrieved] = useState<RetrievedExternalRecord | null>(null);
  const current = operational.find(value => value.Status === "InVisit");
  const detailVisit = props.selected ?? (directoryPatient ? [...mine].reverse().find(value => value.PatientID === directoryPatient) ?? null : null);
  const detailPatient = detailVisit ? patientFor(props.patients, detailVisit) : props.patients.find(value => value.ID === directoryPatient);
  if (detailPatient) return <DoctorPatientDetail {...props} patient={detailPatient} visit={detailVisit} history={mine.filter(value => value.PatientID === detailPatient.ID)} activeVisit={current} retrieved={retrieved} setRetrieved={setRetrieved} back={() => { setRetrieved(null); props.select(null); setDirectoryPatient(null); }} />;
  if (props.path === "/app/patients") return <DoctorPatients patients={props.patients} visits={mine} referrals={props.referrals} open={(patient, visit) => { setRetrieved(null); setDirectoryPatient(patient.ID); props.select(visit); }} />;
  if (props.path === "/app/tasks") return <DoctorTasks visits={operational} patients={props.patients} open={visit => { setRetrieved(null); props.select(visit); }} />;
  return <DoctorHome visits={operational} patients={props.patients} open={visit => { setRetrieved(null); props.select(visit); }} act={props.act} />;
}

function DoctorHome({ visits, patients, open, act }: { visits: Visit[]; patients: Patient[]; open: (visit: Visit) => void; act: DoctorProps["act"] }) {
  const reduce = useReducedMotion();
  const active = visits.find(value => value.Status === "InVisit");
  const waiting = visits.filter(value => value.Status === "Waiting").sort(byArrival);
  const completed = visits.filter(value => value.Status === "Completed" && value.CompletedAt && sameDay(value.CompletedAt)).length;
  const primary = active ?? waiting[0];
  const upNext = active ? waiting : waiting.slice(1);
  return <div className="doctor-home"><header className="doctor-heading"><span className="eyebrow">Dr. B · Hospital B</span><h2>Good morning, Dr. B</h2><p>{waiting.length} {waiting.length === 1 ? "patient" : "patients"} waiting · {active ? 1 : 0} currently in visit</p></header>
    {primary ? <motion.section className="doctor-primary" initial={reduce ? false : { opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}><span className="eyebrow">{active ? "Current Visit" : "Next Patient"}</span><h3>{patientFor(patients, primary)?.FullName ?? primary.PatientID}</h3><p>{primary.Reason}</p><b>Room {primary.Room}</b><small>{active ? `Visit started ${new Date(primary.StartedAt ?? primary.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : `${new Date(primary.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · ${waitingFor(primary.CheckedInAt)}`}</small>{active ? <button className="button" onClick={() => open(primary)}>Continue Visit</button> : <div className="doctor-primary-actions"><button className="button secondary" onClick={() => open(primary)}>View Patient</button><button className="button" onClick={() => void act(() => careApi.start(primary.ID))}>Start Visit</button></div>}</motion.section> : <section className="doctor-caught-up"><h3>You&apos;re caught up.</h3><p>No patients are currently waiting.</p></section>}
    {upNext.length > 0 && <section className="doctor-section"><div className="section-title"><h3>Up Next</h3><span>{upNext.length} waiting</span></div><div className="doctor-queue">{upNext.map(visit => <DoctorQueueRow key={visit.ID} visit={visit} patient={patientFor(patients, visit)} open={() => open(visit)} />)}</div></section>}
    {completed > 0 && <p className="completed-note"><CheckCircle2 /> {completed} completed today</p>}
  </div>;
}

function DoctorQueueRow({ visit, patient, open }: { visit: Visit; patient?: Patient; open: () => void }) {
  const reduce = useReducedMotion();
  return <motion.article className="doctor-queue-row" layout initial={reduce ? false : { opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}><div><b>{patient?.FullName ?? visit.PatientID}</b><span>{visit.Reason}</span></div><div><b>Room {visit.Room}</b><span>{waitingFor(visit.CheckedInAt)}</span></div><button className="button secondary small" onClick={open}>View Patient</button></motion.article>;
}

function DoctorPatients({ patients, visits, referrals, open }: { patients: Patient[]; visits: Visit[]; referrals: Referral[]; open: (patient: Patient, visit: Visit | null) => void }) {
  const [filter, setFilter] = useState<"Today" | "Recent" | "All">("Today"), [query, setQuery] = useState(""), [results, setResults] = useState<Patient[] | null>(null), [loading, setLoading] = useState(false);
  useEffect(() => { const value = query.trim(); if (!value) return; const timer = setTimeout(() => { setLoading(true); void careApi.patients("b", value).then(value => setResults(value ?? [])).catch(() => setResults([])).finally(() => setLoading(false)); }, 300); return () => clearTimeout(timer); }, [query]);
  const visible = (query.trim() ? results ?? [] : patients).filter(patient => { const history = visits.filter(value => value.PatientID === patient.ID); if (filter === "Today") return history.some(value => sameDay(value.CheckedInAt)); if (filter === "Recent") return history.length > 0; return true; });
  const latest = (patient: Patient) => [...visits].filter(value => value.PatientID === patient.ID).sort((a, b) => new Date(b.CheckedInAt).valueOf() - new Date(a.CheckedInAt).valueOf())[0] ?? null;
  return <section className="patient-directory"><header><span className="eyebrow">Doctor workspace</span><h2>My Patients</h2><p>Find patients and review their available visit history.</p></header><label className="doctor-search">Search patients<input value={query} onChange={event => setQuery(event.target.value)} placeholder="Search patients by name, ID or phone..." /></label><div className="directory-filters" role="group" aria-label="Patient history filter">{(["Today", "Recent", "All"] as const).map(value => <button key={value} className={filter === value ? "active" : ""} onClick={() => setFilter(value)}>{value}</button>)}</div>{loading && <p role="status">Searching patients…</p>}{!loading && !visible.length && <div className="directory-empty">No patients match this search and filter.</div>}<div className="patient-directory-list">{visible.map(patient => { const visit = latest(patient), referred = referrals.some(value => value.PatientID === patient.ID); return <article key={patient.ID}><div className="patient-identity"><h3>{patient.FullName}</h3><p>{age(patient.DateOfBirth)} years · {patient.Gender}</p><small>Patient ID {patient.ID}</small></div><div className="patient-context">{visit ? <><span>Last visit: {sameDay(visit.CheckedInAt) ? "Today" : new Date(visit.CheckedInAt).toLocaleDateString()}</span><b>{visit.Reason}</b></> : <span>No visits available</span>}{referred && <small>Referred from Hospital A</small>}</div><button className="button secondary" onClick={() => open(patient, visit)}>View Patient</button></article>})}</div></section>;
}

function DoctorPatientDetail(props: DoctorProps & { patient: Patient; visit: Visit | null; history: Visit[]; activeVisit?: Visit; retrieved: RetrievedExternalRecord | null; setRetrieved: (value: RetrievedExternalRecord | null) => void; back: () => void }) {
  const { patient, visit, retrieved: retrievedState, setRetrieved } = props;
  const local = props.records.filter(value => value.PatientID === patient.ID);
  const referral = props.referrals.find(value => value.NetworkPatientID === patient.NetworkPatientID);
  const request = props.requests.find(value => value.PatientID === patient.ID && (!referral || value.ReferralID === referral.ID));
  const approved = request?.Status === "Granted";
  const sourceOrganizationID = request?.SourceOrganizationID || referral?.SourceOrganizationID || "hospital-a";
  const retrieved = retrievedState && approved && request && retrievedState.patientID === patient.ID && retrievedState.requestID === request.ID && retrievedState.recordID === request.RecordID && retrievedState.sourceOrganizationID === sourceOrganizationID && retrievedState.discoveryScopeID === (request.DiscoveryScopeID || "") ? retrievedState : null;
  const canStart = visit?.Status === "Waiting" && sameDay(visit.CheckedInAt) && (!props.activeVisit || props.activeVisit.ID === visit.ID);
  const [workspace, setWorkspace] = useState(false), [viewRecord, setViewRecord] = useState<string | null>(null), [completedRecord, setCompletedRecord] = useState<string | null>(null), [dirty, setDirty] = useState(false), [leaveOpen, setLeaveOpen] = useState(false);
  const selectedRecord = local.find(value => value.ID === viewRecord) ?? null;
  const linkedVisit = selectedRecord ? props.history.find(value => value.RecordID === selectedRecord.ID) : undefined;
  useEffect(() => { const onUnload = (event: BeforeUnloadEvent) => { if (!dirty) return; event.preventDefault(); event.returnValue = ""; }; window.addEventListener("beforeunload", onUnload); return () => window.removeEventListener("beforeunload", onUnload); }, [dirty]);
  useEffect(() => { if (retrievedState && !retrieved) setRetrieved(null); }, [retrievedState, setRetrieved, retrieved]);
  const leave = () => { if (dirty) setLeaveOpen(true); else { setWorkspace(false); props.back(); } };
  const start = () => { if (!visit) return; void props.act(async () => { await careApi.start(visit.ID); setWorkspace(true); }); };
  const history = <RelevantHistory patient={patient} visits={props.history} local={local} referral={referral} request={request} approved={approved} external={retrieved} setExternal={setRetrieved} act={props.act} openRecord={setViewRecord} />;
  if (selectedRecord) return <div className="doctor-patient-detail"><button className="text-action" onClick={() => setViewRecord(null)}>← {patient.FullName}</button><PatientHeader patient={patient}/><ClinicalRecordView record={selectedRecord} visit={linkedVisit}/></div>;
  if (workspace && visit?.Status === "InVisit") return <motion.div className="doctor-patient-detail visit-workspace" initial={{ opacity: 0, y: 5 }} animate={{ opacity: 1, y: 0 }}><button className="text-action" onClick={leave}>← Patient Detail</button><PatientHeader patient={patient} compact/><section className="visit-progress"><span className="eyebrow">Visit in progress</span><h3>{visit.Reason}</h3><p>Room {visit.Room} · Started {new Date(visit.StartedAt ?? visit.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</p></section><div className="visit-workspace-grid"><VisitDocumentation visit={visit} act={props.act} busy={props.busy} onDirty={setDirty} onComplete={record => { setDirty(false); setCompletedRecord(record.ID); setViewRecord(record.ID); setWorkspace(false); }}/><aside className="visit-reference" aria-label="Relevant patient history">{history}</aside></div>{leaveOpen&&<ConfirmDialog title="You have unsaved changes." description="Your latest documentation has not been saved." confirm="Leave without saving" onCancel={() => setLeaveOpen(false)} onConfirm={() => { setDirty(false); setLeaveOpen(false); setWorkspace(false); props.back(); }}/>}</motion.div>;
  return <div className="doctor-patient-detail"><button className="text-action" onClick={props.back}>← My Patients</button><PatientHeader patient={patient}/>
    {completedRecord && <motion.div className="visit-success" role="status" initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}><CheckCircle2/><div><b>Visit completed</b><span>New medical record created at Hospital B.</span></div><button className="button secondary small" onClick={() => setViewRecord(completedRecord)}>View Record</button></motion.div>}
    {visit && <section className="today-visit"><span className="eyebrow">Today&apos;s Visit</span><strong className={`visit-state ${visit.Status.toLowerCase()}`}>{visit.Status === "InVisit" ? "In Visit" : visit.Status}</strong><h3>{visit.Reason}</h3><p>Room {visit.Room}</p><small>{visit.Status === "Waiting" ? `Checked in ${new Date(visit.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · ${waitingFor(visit.CheckedInAt)}` : visit.Status === "InVisit" ? `Started ${new Date(visit.StartedAt ?? visit.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : `Completed at ${new Date(visit.CompletedAt ?? visit.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · Clinical record created`}</small>{canStart && <button data-testid="start-visit" className="button" onClick={start}>Start Visit</button>}{visit.Status === "Waiting" && !canStart && <p className="active-warning">Complete the current visit before starting another.</p>}{visit.Status === "InVisit" && <button className="button" onClick={() => setWorkspace(true)}>Continue Visit</button>}{visit.Status === "Completed" && visit.RecordID && <button className="button secondary" onClick={() => setViewRecord(visit.RecordID!)}>View Record</button>}</section>}
    <section className="relevant-history"><div className="section-title"><div><span className="eyebrow">Relevant history</span><h3>Clinical context</h3></div></div><div className="patient-detail-grid">{history}</div></section></div>;
}

function PatientHeader({ patient, compact = false }: { patient: Patient; compact?: boolean }) { return <header className={compact ? "patient-header compact" : "patient-header"}><div><h2>{patient.FullName}</h2><p>{age(patient.DateOfBirth)} years · {patient.Gender}</p><small>Patient ID: {patient.ID}<span>Phone: {patient.Phone}</span></small></div></header>; }

function RelevantHistory({ patient, visits, local, referral, request: possibleRequest, approved, external, setExternal, act, openRecord }: { patient: Patient; visits: Visit[]; local: ClinicalRecord[]; referral?: Referral; request?: AccessRequest; approved: boolean; external: RetrievedExternalRecord | null; setExternal: (value: RetrievedExternalRecord | null) => void; act: DoctorProps["act"]; openRecord: (id: string) => void }) {
  const request = possibleRequest as AccessRequest;
  const [discovery, setDiscovery] = useState<RecordDiscovery | null>(null);
  const [discoveryFailed, setDiscoveryFailed] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (referral) return;
    let current = true;
    void accessApi.discover(patient.ID).then(value => { if (current) setDiscovery(value); }).catch(() => { if (current) setDiscoveryFailed(true); });
    return () => { current = false; };
  }, [patient.ID, referral, retry]);
  const checking = !referral && discovery === null && !discoveryFailed;
  const available = Boolean(referral || discovery?.RecordsAvailable);
  const requestAccess = () => {
    const now = new Date().toISOString();
    const scope = discovery?.Scopes[0];
    return accessApi.request({ ID: newID(), ReferralID: referral?.ID ?? "", PatientID: patient.ID, NetworkPatientID: patient.NetworkPatientID ?? patient.ID, RecordID: referral?.RecordID ?? "", DiscoveryScopeID: scope?.ID ?? "", RequesterActorID: "doctor-b", RequesterOrganizationID: "hospital-b", SourceOrganizationID: referral?.SourceOrganizationID ?? discovery?.OrganizationID ?? "hospital-a", Purpose: "Continuity of care", Status: "", CreatedAt: now, UpdatedAt: now });
  };
  return <>
    <section className="clinical-section"><h3>Patient Summary</h3><dl><div><dt>Date of birth</dt><dd>{patient.DateOfBirth}</dd></div><div><dt>Age</dt><dd>{age(patient.DateOfBirth)} years</dd></div><div><dt>Gender</dt><dd>{patient.Gender}</dd></div><div><dt>Phone</dt><dd>{patient.Phone}</dd></div></dl></section>
    <section className="clinical-section"><h3>Recent Visits</h3>{visits.length ? [...visits].sort((a, b) => new Date(b.CheckedInAt).valueOf() - new Date(a.CheckedInAt).valueOf()).map(value => <article className="history-row" key={value.ID}><time>{new Date(value.CheckedInAt).toLocaleDateString()}</time><div><b>{value.Reason}</b><span>Hospital B · {value.Status === "InVisit" ? "In Visit" : value.Status}{value.RecordID ? " · Clinical record created" : ""}</span></div></article>) : <p>No previous visits available.</p>}</section>
    <section className="clinical-section local-records"><h3>Local Records</h3><div className="record-ownership"><b>Hospital B</b><span>Created by your hospital · Directly available</span></div>{local.length ? local.map(value => <article className="clinical-record" key={value.ID}><b>{value.Diagnosis}</b><span>{new Date(value.CreatedAt).toLocaleString()} · Dr. B</span><small><ShieldCheck /> Integrity protected</small><button className="text-action" onClick={() => openRecord(value.ID)}>View Record</button></article>) : <div className="clinical-empty"><b>No local records yet.</b><span>A record will appear here after a completed visit.</span></div>}</section>
    <section className="clinical-section external-section" data-testid="external-records"><h3>External Records</h3>
      {checking && <div className="clinical-empty"><b>Checking trusted hospitals…</b></div>}
      {discoveryFailed && <div className="clinical-empty"><b>We couldn&apos;t check external records right now.</b><span>Trusted hospital history is temporarily unavailable.</span><button className="button secondary small" onClick={() => { setDiscovery(null); setDiscoveryFailed(false); setRetry(value => value + 1); }}>Retry</button></div>}
      {!checking && !discoveryFailed && !available && <div className="clinical-empty"><b>No external records found.</b></div>}
      {!checking && available && <><div className="record-ownership external-source"><b>Hospital A</b><span>Previous records are available for this patient.</span></div><p>The original records remain at Hospital A.</p>
        {!request && <><p>{referral ? "External record access requires patient approval." : "Request access only if this history is clinically relevant."}</p><button data-testid="request-access" className="button" onClick={() => void act(requestAccess)}>Request Access</button></>}
        {request && !approved && <div className="external-status"><LockKeyhole /><b>{request.Status === "Revoked" ? "Access revoked" : request.Status === "Denied" ? "Access declined" : "Approval pending"}</b></div>}
        {approved && !external && <><p>{referral ? "Available securely" : "Access approved"}</p><button data-testid="open-external-record" className="button" onClick={() => void act(async () => { setExternal(null); const record = await clinicalApi.retrieve(request.RecordID, request.ReferralID, request.ID); const check = await clinicalApi.verifyShared(record); if (!check.integrityValid || record.ID !== request.RecordID) throw new Error("integrity"); setExternal({ record, patientID: patient.ID, requestID: request.ID, recordID: record.ID, sourceOrganizationID: request.SourceOrganizationID || referral?.SourceOrganizationID || discovery?.OrganizationID || "hospital-a", discoveryScopeID: request.DiscoveryScopeID || "" }); })}>View Record</button></>}
        {external && <div className="external-record" data-testid="external-record"><div className="integrity-ok"><CheckCircle2 /> Integrity verified</div><p><b>Encounter:</b> {external.record.EncounterSummary}</p><p><b>Diagnosis:</b> {external.record.Diagnosis}</p><p><b>Prescription:</b> {external.record.Prescription}</p><small>This record remains stored at Hospital A.</small></div>}
      </>}
    </section>
  </>;
}

type DraftState = { ChiefComplaint: string; ClinicalNotes: string; Diagnosis: string; Prescription: string; FollowUpPlan: string };
const draftFrom = (visit: Visit): DraftState => ({ ChiefComplaint: visit.ChiefComplaint || visit.Reason, ClinicalNotes: visit.ClinicalNotes || "", Diagnosis: visit.Diagnosis || "", Prescription: visit.Prescription || "", FollowUpPlan: visit.FollowUpPlan || "" });
const draftKey = (value: DraftState) => JSON.stringify(value);

function VisitDocumentation({ visit, act, busy, onDirty, onComplete }: { visit: Visit; act: DoctorProps["act"]; busy: boolean; onDirty: (dirty: boolean) => void; onComplete: (record: ClinicalRecord) => void }) {
  const [draft, setDraft] = useState<DraftState>(() => draftFrom(visit)), [baseline, setBaseline] = useState(() => draftFrom(visit)), [savedAt, setSavedAt] = useState<Date | null>(null), [confirming, setConfirming] = useState(false);
  const dirty = draftKey(draft) !== draftKey(baseline);
  useEffect(() => { onDirty(dirty); }, [dirty, onDirty]);
  const update = (field: keyof DraftState, value: string) => setDraft(current => ({ ...current, [field]: value }));
  const save = () => void act(async () => { await careApi.draft(visit.ID, draft); setBaseline(draft); setSavedAt(new Date()); });
  const complete = () => void act(async () => { const record = await careApi.complete(visit.ID, newID(), draft) as ClinicalRecord; setBaseline(draft); setConfirming(false); onComplete(record); });
  return <section className="clinical-section visit-documentation" id="visit-documentation"><span className="eyebrow">Current examination</span><h3>Visit Documentation</h3><form className="record-form" data-testid="visit-documentation" onSubmit={event => { event.preventDefault(); save(); }}><label>Chief Complaint<input name="complaint" value={draft.ChiefComplaint} onChange={event => update("ChiefComplaint", event.target.value)} /></label><label>Clinical Notes<textarea className="clinical-notes" name="notes" value={draft.ClinicalNotes} onChange={event => update("ClinicalNotes", event.target.value)} /></label><label>Diagnosis<input name="diagnosis" value={draft.Diagnosis} onChange={event => update("Diagnosis", event.target.value)} /></label><label>Treatment / Plan<textarea name="followup" value={draft.FollowUpPlan} onChange={event => update("FollowUpPlan", event.target.value)} /></label><label>Prescription / Medication <small>Optional</small><input name="prescription" value={draft.Prescription} onChange={event => update("Prescription", event.target.value)} /></label>{savedAt && <motion.p className="draft-saved" role="status" initial={{ opacity: 0 }} animate={{ opacity: 1 }}><CheckCircle2/> Draft saved · {savedAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</motion.p>}<div className="visit-action-bar"><button className="button secondary" disabled={busy} type="submit">Save Draft</button><button data-testid="complete-visit" type="button" className="button" disabled={busy} onClick={() => setConfirming(true)}>Complete Visit</button></div></form>{confirming && <ConfirmDialog title="Complete this visit?" description="The current examination will be finalized and saved as a new Hospital B clinical record. This action will mark the visit as completed." confirm="Complete Visit" onCancel={() => setConfirming(false)} onConfirm={complete}/>}</section>;
}

function ConfirmDialog({ title, description, confirm, onCancel, onConfirm }: { title: string; description: string; confirm: string; onCancel: () => void; onConfirm: () => void }) {
  const dialog = useRef<HTMLDivElement>(null), cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => { const previous = document.activeElement as HTMLElement | null; cancel.current?.focus(); const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") { event.preventDefault(); onCancel(); return; } if (event.key !== "Tab" || !dialog.current) return; const items = [...dialog.current.querySelectorAll<HTMLElement>('button,[href],input,textarea,select,[tabindex]:not([tabindex="-1"])')]; if (!items.length) return; const first = items[0], last = items.at(-1)!; if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); } else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); } }; document.addEventListener("keydown", onKey); return () => { document.removeEventListener("keydown", onKey); previous?.focus(); }; }, [onCancel]);
  return <div className="dialog-backdrop" role="presentation"><div ref={dialog} className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-description"><button className="dialog-close" onClick={onCancel} aria-label="Close confirmation"><X/></button><span className="dialog-icon"><FileHeart/></span><h3 id="confirm-title">{title}</h3><p id="confirm-description">{description}</p><div><button ref={cancel} className="button secondary" onClick={onCancel}>Cancel</button><button className="button" onClick={onConfirm}>{confirm}</button></div></div></div>;
}

function ClinicalRecordView({ record, visit }: { record: ClinicalRecord; visit?: Visit }) {
  const [complaint, notes] = record.EncounterSummary.split(" — ", 2);
  return <motion.section className="clinical-record-view" data-testid="local-record" initial={{ opacity: 0, y: 5 }} animate={{ opacity: 1, y: 0 }}><header><span className="eyebrow">Clinical Record</span><h3>Hospital B</h3><p>Dr. B · {new Date(record.CreatedAt).toLocaleString()}</p><span className="record-integrity"><ShieldCheck/> Integrity protected · Verified</span></header><dl><div><dt>Chief complaint</dt><dd>{visit?.ChiefComplaint || complaint || "Not recorded"}</dd></div><div><dt>Clinical notes</dt><dd>{visit?.ClinicalNotes || notes || record.EncounterSummary}</dd></div><div><dt>Diagnosis</dt><dd>{record.Diagnosis}</dd></div><div><dt>Treatment / Plan</dt><dd>{visit?.FollowUpPlan || "No treatment plan recorded."}</dd></div><div><dt>Prescription / Medication</dt><dd>{record.Prescription || "No prescription recorded."}</dd></div></dl></motion.section>;
}

function DoctorTasks({ visits, patients, open }: { visits: Visit[]; patients: Patient[]; open: (visit: Visit) => void }) { const tasks = visits.filter(value => value.Status !== "Completed"); return <section className="doctor-section"><div className="section-title"><h2>Tasks</h2><span>{tasks.length} open</span></div>{tasks.length ? tasks.map(value => <DoctorQueueRow key={value.ID} visit={value} patient={patientFor(patients, value)} open={() => open(value)} />) : <div className="doctor-caught-up"><h3>You&apos;re caught up.</h3><p>No clinical tasks are currently open.</p></div>}</section>; }
