"use client";

import { motion, useReducedMotion } from "framer-motion";
import { useEffect, useRef, useState } from "react";
import { careApi, type Patient, type Visit } from "../lib/api/operations-workflow";
import type { Referral } from "../lib/api/referrals";

export const receptionRooms = ["203", "204", "205"] as const;
export const visitReasons = ["General consultation", "Follow-up", "New symptoms", "Test / results review", "Medication / prescription", "Post-treatment follow-up", "Other"] as const;

type ModalProps = { referrals: Referral[]; busy: boolean; initialPatient: Patient | null; close: () => void; onSuccess: (patient: Patient, room: string) => void; act: (fn: () => Promise<unknown>) => Promise<void> };

export function CheckInModal({ referrals, busy, initialPatient, close, onSuccess, act }: ModalProps) {
  const [mode, setMode] = useState<"find" | "register" | null>(initialPatient ? "find" : null);
  const [selected, setSelected] = useState<Patient | null>(initialPatient);
  const [doctor, setDoctor] = useState("doctor-b");
  const [room, setRoom] = useState("");
  const [reason, setReason] = useState("");
  const [otherReason, setOtherReason] = useState("");
  const [referralID, setReferralID] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const modal = useRef<HTMLDivElement>(null);
  const closeRef = useRef(close);
  const reduce = useReducedMotion();
  const applicable = selected ? referrals.filter(value => value.NetworkPatientID === selected.NetworkPatientID) : [];
  const finalReason = reason === "Other" ? otherReason.trim() : reason;
  const valid = Boolean(selected && doctor && room && finalReason);

  useEffect(() => { closeRef.current = close; }, [close]);

  useEffect(() => {
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const element = modal.current;
    element?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeRef.current();
      if (event.key !== "Tab" || !element) return;
      const focusable = [...element.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled])')];
      if (!focusable.length) return;
      const first = focusable[0], last = focusable.at(-1)!;
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("keydown", onKey); returnFocus?.focus(); };
  }, []);

  const submit = async () => {
    if (!selected || !valid || submitting) return;
    setSubmitting(true);
    let completed = false;
    await act(async () => {
      await careApi.checkIn({ ID: crypto.randomUUID(), PatientID: selected.ID, OrganizationID: "", DoctorID: doctor, Room: room, Reason: finalReason, ReferralID: referralID, Source: referralID ? "Referral" : "Normal", Status: "Waiting", CheckedInAt: new Date().toISOString() });
      completed = true;
    });
    setSubmitting(false);
    if (completed) onSuccess(selected, room);
  };

  return <div className="modal-layer"><button className="modal-scrim" aria-label="Close check-in" onClick={close} /><motion.div ref={modal} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby="checkin-title" className="checkin-modal" initial={reduce ? false : { scale: .98, opacity: 0, y: 8 }} animate={{ scale: 1, opacity: 1, y: 0 }}>
    <header><h2 id="checkin-title">Check In Patient</h2><button className="modal-close" aria-label="Close check-in" onClick={close}>×</button></header>
    <div className="modal-progress" aria-label={selected ? "Step 2 of 2: Visit Details" : "Step 1 of 2: Patient"}><span className="active">Patient</span><i /><span className={selected ? "active" : ""}>Visit Details</span></div>
    <div className="modal-body">
      {!selected && mode === null && <section className="patient-choice"><h3>What do you want to do?</h3><button className="button wide" onClick={() => setMode("find")}>Find Existing Patient</button><button className="button secondary wide" onClick={() => setMode("register")}>Register New Patient</button></section>}
      {!selected && mode === "find" && <><ReceptionPatientSearch onSelect={value => { setSelected(value); setReferralID(""); }} /><button className="text-action" onClick={() => setMode(null)}>← Back</button></>}
      {!selected && mode === "register" && <RegistrationForm busy={busy} act={act} onCancel={() => setMode(null)} onRegistered={value => setSelected(value)} />}
      {selected && <><section className="selected-patient"><span className="eyebrow">Patient selected</span><PatientCard patient={selected} /><button className="button secondary small" onClick={() => { setSelected(null); setMode(null); setReferralID(""); }}>Change Patient</button></section>
        <div className="checkin-fields"><label>Assigned doctor<select value={doctor} onChange={event => setDoctor(event.target.value)}><option value="doctor-b">Dr. B</option></select></label><label>Room<select value={room} onChange={event => setRoom(event.target.value)} required><option value="">Select a room</option>{receptionRooms.map(value => <option key={value} value={value}>Room {value}</option>)}</select></label><label className="full-field">Visit reason<select value={reason} onChange={event => setReason(event.target.value)} required><option value="">Select a reason</option>{visitReasons.map(value => <option key={value} value={value}>{value}</option>)}</select></label>{reason === "Other" && <label className="full-field">Please specify<input value={otherReason} onChange={event => setOtherReason(event.target.value)} required /></label>}</div>
        {applicable.length ? <section className="referral-options"><h3>Referral available</h3>{applicable.map(value => <motion.article key={value.ID} initial={reduce ? false : { opacity: 0 }} animate={{ opacity: 1 }}><b>Hospital A → Hospital B</b><p>Referred for continued care</p><small>Received: {new Date(value.CreatedAt).toLocaleDateString()}</small><button className={referralID === value.ID ? "button" : "button secondary"} onClick={() => setReferralID(value.ID)}>{referralID === value.ID ? "Referral selected" : "Use Referral"}</button></motion.article>)}<button className="text-action" onClick={() => setReferralID("")}>Continue as Regular Visit</button></section> : <section className="regular-appointment"><b>Regular visit</b></section>}
      </>}
    </div>
    {selected && <footer><button className="button wide" disabled={!valid || busy || submitting} onClick={() => void submit()}>{submitting ? "Checking in…" : "Check In Patient"}</button></footer>}
  </motion.div></div>;
}

function RegistrationForm({ busy, act, onCancel, onRegistered }: { busy: boolean; act: ModalProps["act"]; onCancel: () => void; onRegistered: (patient: Patient) => void }) {
  const [submitting, setSubmitting] = useState(false);
  return <form className="registration-form" data-testid="patient-registration" onSubmit={event => { event.preventDefault(); const form = new FormData(event.currentTarget), fullName = String(form.get("name")), generatedID = fullName.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || `patient-${crypto.randomUUID()}`, networkID = String(form.get("network-id")).trim(); const patient: Patient = { ID: generatedID, OrganizationID: "", NetworkPatientID: networkID || generatedID, FullName: fullName, DateOfBirth: String(form.get("dob")), Gender: String(form.get("gender")), Phone: String(form.get("phone")) }; setSubmitting(true); let completed = false; void act(async () => { await careApi.register("b", patient); completed = true; }).then(() => { setSubmitting(false); if (completed) onRegistered(patient); }); }}>
    <h3>Register New Patient</h3><div className="registration-grid"><label>Full Name<input name="name" autoComplete="name" required /></label><label>Date of Birth<input name="dob" type="date" required /></label><fieldset className="full-field"><legend>Gender</legend><label><input type="radio" name="gender" value="Female" required /> Female</label><label><input type="radio" name="gender" value="Male" /> Male</label><label><input type="radio" name="gender" value="Other / Prefer not to say" /> Other / Prefer not to say</label></fieldset><label className="full-field">Phone<input name="phone" type="tel" autoComplete="tel" required /></label><label className="full-field">Existing care network ID <small>Optional prototype correlation; use only with a confirmed network registration ID.</small><input name="network-id" autoComplete="off" /></label></div><button className="button wide" disabled={busy || submitting}>{submitting ? "Registering…" : "Register and Continue"}</button><button type="button" className="text-action" onClick={onCancel}>← Back</button>
  </form>;
}

export function ReceptionPatientSearch({ onSelect, onCheckIn, onRegister, selected }: { onSelect: (patient: Patient) => void; onCheckIn?: (patient: Patient) => void; onRegister?: () => void; selected?: string }) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Patient[]>([]);
  const [loading, setLoading] = useState(false);
  const reduce = useReducedMotion();
  useEffect(() => {
    const value = query.trim();
    if (!value) return;
    const timer = setTimeout(() => { setLoading(true); void careApi.patients("b", value).then(value => setResults(value ?? [])).catch(() => setResults([])).finally(() => setLoading(false)); }, 300);
    return () => clearTimeout(timer);
  }, [query]);
  return <section className="patient-search" aria-label="Find a patient">
    <label><span>Find a patient</span><input value={query} onChange={event => setQuery(event.target.value)} placeholder="Search patient by name, ID or phone..." /></label>
    {loading && <p role="status">Searching patients…</p>}
    {query && !loading && !results.length && <div className="search-empty"><p>No matching patient found.</p><button className="button secondary" onClick={onRegister ?? (() => { window.location.href = "/app/check-in"; })}>Register New Patient</button></div>}
    <motion.div className="search-results" layout>{results.map(value => <motion.div key={value.ID} className={selected === value.ID ? "patient-result selected" : "patient-result"} initial={reduce ? false : { opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}><PatientCard patient={value} /><div><button className="button secondary" onClick={() => onSelect(value)}>Select Patient</button>{onCheckIn && <button className="button" onClick={() => onCheckIn(value)}>Check In</button>}</div></motion.div>)}</motion.div>
  </section>;
}

export function PatientCard({ patient }: { patient: Patient }) {
  const birth = new Date(patient.DateOfBirth);
  const age = Number.isNaN(birth.valueOf()) ? null : Math.max(0, new Date().getFullYear() - birth.getFullYear());
  return <div className="patient-card-compact"><b>{patient.FullName}</b><span>Patient ID: {patient.ID}</span><span>{patient.DateOfBirth}{age === null ? "" : ` · ${age} years`}</span><span>{patient.Phone}</span></div>;
}

export function ReceptionQueue({ visits, patients, open, checkIn }: { visits: Visit[]; patients: Patient[]; referrals: Referral[]; open: (visit: Visit) => void; checkIn: () => void }) {
  const reduce = useReducedMotion();
  const now = new Date();
  visits = visits.filter(value => { const date = new Date(value.CheckedInAt); return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate(); });
  if (!visits.length) return <div className="queue-empty"><p>No patients have checked in yet.</p><button className="button secondary" onClick={checkIn}>Check In Patient</button></div>;
  return <motion.div className="reception-queue" layout>{visits.map(visit => {
    const patient = patients.find(value => value.ID === visit.PatientID);
    const referred = visit.Source === "Referral" || Boolean(visit.ReferralID);
    return <motion.article className="queue-row" layout key={visit.ID} initial={reduce ? false : { opacity: 0, y: 5 }} animate={{ opacity: 1, y: 0 }}>
      <time>{new Date(visit.CheckedInAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
      <div className="queue-patient"><b>{patient?.FullName ?? visit.PatientID}</b><span>{visit.Reason}</span></div>
      <div><b>Dr. B · Room {visit.Room}</b><span className={referred ? "source referred" : "source regular"}>{referred ? "Referred from Hospital A" : "Regular visit"}</span></div>
      <strong className={`status ${visit.Status.toLowerCase()}`}>{visit.Status === "InVisit" ? "In Visit" : visit.Status}</strong>
      <button className="button secondary small" onClick={() => open(visit)}>View</button>
    </motion.article>;
  })}</motion.div>;
}

export function ReceptionVisit({ visit, patient, referral, back }: { visit: Visit; patient?: Patient; referral?: Referral; back: () => void }) {
  return <><button className="button secondary" onClick={back}>← Today&apos;s Patients</button><section className="panel section-page"><div className="panel-head"><h2>Reception patient view</h2></div><div className="reception-patient-view">{patient && <PatientCard patient={patient} />}<h3>Today&apos;s visit</h3><dl><div><dt>Arrival</dt><dd>{new Date(visit.CheckedInAt).toLocaleTimeString()}</dd></div><div><dt>Doctor</dt><dd>Dr. B</dd></div><div><dt>Room</dt><dd>{visit.Room}</dd></div><div><dt>Reason</dt><dd>{visit.Reason}</dd></div><div><dt>Status</dt><dd>{visit.Status === "InVisit" ? "In Visit" : visit.Status}</dd></div><div><dt>Visit source</dt><dd>{visit.Source === "Referral" ? "Referred from Hospital A" : "Regular visit"}</dd></div>{referral && <div><dt>Referral status</dt><dd>{referral.Status}</dd></div>}</dl></div></section></>;
}
