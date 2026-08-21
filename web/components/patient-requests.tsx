"use client";

import { motion, useReducedMotion } from "framer-motion";
import { CheckCircle2, Clock3, FileHeart, LockKeyhole, ShieldCheck, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { idempotencyKey, newID } from "../lib/api/client";
import { consentApi, type Consent } from "../lib/api/consent";
import type { AccessRequest } from "../lib/api/access";

type PatientRequestsProps = {
  requests: AccessRequest[];
  outgoing: AccessRequest[];
  consents: Consent[];
  act: (operation: () => Promise<unknown>) => Promise<void>;
};

type Notice = { title: string; detail: string };

export function PatientRequests({ requests, outgoing, consents, act }: PatientRequestsProps) {
  const reduce = useReducedMotion();
  const [reviewing, setReviewing] = useState<AccessRequest | null>(null);
  const [revoking, setRevoking] = useState<{ request: AccessRequest; consent: Consent } | null>(null);
  const [notice, setNotice] = useState<Notice | null>(null);
  const combined = deduplicate([...outgoing, ...requests]).sort((a, b) => new Date(b.CreatedAt).valueOf() - new Date(a.CreatedAt).valueOf());
  const consentFor = (request: AccessRequest) => consents.find(value => value.AccessRequestID === request.ID);
  const attention = combined.filter(request => request.Status === "Pending" && !consentFor(request));
  const active = combined.flatMap(request => {
    const consent = consentFor(request);
    return consent?.Status === "Active" ? [{ request, consent }] : [];
  });
  const past = combined.filter(request => request.Status === "Denied" || request.Status === "Revoked" || consentFor(request)?.Status === "Revoked");

  const allow = async (request: AccessRequest) => {
    let completed = false;
    await act(async () => { await consentApi.grant(request.ID, newID(), request.PatientID, idempotencyKey("allow")); completed = true; });
    if (completed) {
      setReviewing(null);
      setNotice({ title: "Access approved", detail: "Dr. B can now securely view the requested Hospital A record. Your original record remains stored at Hospital A." });
    }
  };
  const decline = async (request: AccessRequest) => {
    let completed = false;
    await act(async () => { await consentApi.decline(request.ID, request.PatientID); completed = true; });
    if (completed) {
      setReviewing(null);
      setNotice({ title: "Request declined", detail: "Dr. B cannot access the requested record." });
    }
  };
  const revoke = async (request: AccessRequest, consent: Consent) => {
    let completed = false;
    await act(async () => { await consentApi.revoke(consent.ID, request.PatientID, idempotencyKey("revoke")); completed = true; });
    if (completed) {
      setRevoking(null);
      setNotice({ title: "Access revoked", detail: "Dr. B can no longer retrieve the requested record." });
    }
  };

  return <main className="patient-requests"><header className="patient-requests-heading"><span className="eyebrow">Your privacy choices</span><h1>Record Access</h1><p>Review and manage who can securely view your Hospital A record.</p></header>
    {notice && <motion.div className="patient-request-notice" role="status" initial={reduce ? false : { opacity: 0, y: -5 }} animate={{ opacity: 1, y: 0 }}><CheckCircle2/><div><b>{notice.title}</b><span>{notice.detail}</span></div><button aria-label="Dismiss confirmation" onClick={() => setNotice(null)}><X/></button></motion.div>}
    <section aria-labelledby="attention-title"><SectionHeading id="attention-title" title="Needs Your Attention" detail={attention.length ? "A decision is waiting for you." : "Nothing needs your decision right now."}/>{attention.length ? <div className="patient-request-grid">{attention.map((request, index) => <RequestCard key={request.ID} request={request} status="Waiting for your decision" delay={index * .04}><button className="button" onClick={() => setReviewing(request)}>Review Request</button></RequestCard>)}</div> : <EmptyState icon={<CheckCircle2/>} title="You're all caught up" detail="There are no access requests waiting for your decision."/>}</section>
    <section aria-labelledby="active-title"><SectionHeading id="active-title" title="Active Access" detail={active.length ? "Access you have currently approved." : "No active external access."}/>{active.length ? <div className="patient-request-grid">{active.map(({ request, consent }) => <AccessCard key={request.ID} request={request} approvedAt={consent.GrantedAt}><button className="button danger" onClick={() => setRevoking({ request, consent })}>Revoke Access</button></AccessCard>)}</div> : <EmptyState icon={<LockKeyhole/>} title="No active external access" detail="No one outside Hospital A currently has approved access to this referred record."/>}</section>
    <section aria-labelledby="past-title"><SectionHeading id="past-title" title="Past Requests" detail="Requests that are no longer active."/>{past.length ? <div className="past-request-list">{past.map(request => <PastRequest key={request.ID} request={request} consent={consentFor(request)}/>)}</div> : <p className="patient-section-empty">No past requests.</p>}</section>
    {reviewing && <ReviewDialog request={reviewing} close={() => setReviewing(null)} allow={() => void allow(reviewing)} decline={() => void decline(reviewing)}/>}
    {revoking && <RevokeDialog close={() => setRevoking(null)} confirm={() => void revoke(revoking.request, revoking.consent)}/>}
  </main>;
}

function SectionHeading({ id, title, detail }: { id: string; title: string; detail: string }) {
  return <div className="patient-request-section-heading"><div><h2 id={id}>{title}</h2><p>{detail}</p></div></div>;
}

function RequestCard({ request, status, delay, children }: { request: AccessRequest; status: string; delay: number; children: React.ReactNode }) {
  const reduce = useReducedMotion();
  return <motion.article className="patient-request-card" initial={reduce ? false : { opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ delay }}><div className="request-card-top"><span className="request-avatar">DB</span><div><h3>Dr. B</h3><p>Hospital B</p></div><span className="request-status pending"><Clock3/>{status}</span></div><dl><div><dt>Would like to view</dt><dd>Your {request.ReferralID ? "referred " : ""}medical record from Hospital A</dd></div><div><dt>Reason</dt><dd>{request.Purpose || "Continuity of care"}</dd></div><div><dt>Requested</dt><dd>{formatDate(request.CreatedAt)}</dd></div></dl><div className="request-card-actions">{children}</div></motion.article>;
}

function AccessCard({ request, approvedAt, children }: { request: AccessRequest; approvedAt: string; children: React.ReactNode }) {
  return <article className="patient-request-card active"><div className="request-card-top"><span className="request-avatar"><ShieldCheck/></span><div><h3>Dr. B</h3><p>Hospital B</p></div><span className="request-status approved"><CheckCircle2/>Access approved</span></div><dl><div><dt>Can securely view</dt><dd>Hospital A {request.ReferralID ? "referred " : ""}record</dd></div><div><dt>Approved</dt><dd>{formatDate(approvedAt || request.UpdatedAt)}</dd></div></dl><p className="access-scope">Dr. B at Hospital B currently has approved access to this specific Hospital A record.</p><div className="request-card-actions">{children}</div></article>;
}

function PastRequest({ request, consent }: { request: AccessRequest; consent?: Consent }) {
  const revoked = request.Status === "Revoked" || consent?.Status === "Revoked";
  return <article><span className={`request-status ${revoked ? "revoked" : "declined"}`}>{revoked ? "Access revoked" : "Request declined"}</span><div><h3>Dr. B · Hospital B</h3><p>Hospital A {request.ReferralID ? "referred " : ""}record · Requested {formatDate(request.CreatedAt)}</p></div></article>;
}

function EmptyState({ icon, title, detail }: { icon: React.ReactNode; title: string; detail: string }) {
  return <div className="patient-request-empty"><span>{icon}</span><div><b>{title}</b><p>{detail}</p></div></div>;
}

function ReviewDialog({ request, close, allow, decline }: { request: AccessRequest; close: () => void; allow: () => void; decline: () => void }) {
  return <PatientDialog title="Dr. B at Hospital B is requesting access" description="Review exactly what you are approving." close={close}><div className="review-request-details"><dl><div><dt>Record</dt><dd>Hospital A clinical record</dd></div><div><dt>Purpose</dt><dd>Continuity of care</dd></div><div><dt>Requested</dt><dd>{formatDate(request.CreatedAt)}</dd></div></dl><div className="approval-explanation"><h3>What happens if you allow access?</h3><ul><li>Dr. B can securely view this record.</li><li>Your original record remains stored at Hospital A.</li><li>Hospital B does not receive ownership of the original record.</li><li>You can revoke access later.</li></ul></div></div><div className="patient-dialog-actions"><button className="button secondary" onClick={decline}>Decline</button><button data-testid="allow-access" className="button" onClick={allow}>Allow Access</button></div></PatientDialog>;
}

function RevokeDialog({ close, confirm }: { close: () => void; confirm: () => void }) {
  return <PatientDialog title="Revoke Dr. B's access?" description="Dr. B will no longer be able to retrieve the Hospital A record." close={close}><div className="revoke-explanation"><p>This does not delete:</p><ul><li>Your Hospital A record</li><li>Your Hospital B visit</li><li>Any previously recorded audit history</li></ul></div><div className="patient-dialog-actions"><button data-autofocus className="button secondary" onClick={close}>Cancel</button><button data-testid="confirm-revoke-access" className="button danger" onClick={confirm}>Revoke Access</button></div></PatientDialog>;
}

function PatientDialog({ title, description, close, children }: { title: string; description: string; close: () => void; children: React.ReactNode }) {
  const dialog = useRef<HTMLDivElement>(null);
  const closeRef = useRef(close);
  const reduce = useReducedMotion();
  useEffect(() => { closeRef.current = close; }, [close]);
  useEffect(() => {
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const root = dialog.current;
    (root?.querySelector<HTMLElement>("[data-autofocus]") ?? root)?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeRef.current();
      if (event.key !== "Tab" || !root) return;
      const controls = [...root.querySelectorAll<HTMLElement>('button,[href],input,select,textarea,[tabindex]:not([tabindex="-1"])')].filter(value => !value.hasAttribute("disabled"));
      if (!controls.length) return;
      const first = controls[0], last = controls.at(-1)!;
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("keydown", onKey); returnFocus?.focus(); };
  }, []);
  return <div className="patient-dialog-layer"><button className="patient-dialog-scrim" aria-label="Close request details" onClick={close}/><motion.div ref={dialog} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby="patient-dialog-title" aria-describedby="patient-dialog-description" className="patient-dialog" initial={reduce ? false : { opacity: 0, scale: .985, y: 6 }} animate={{ opacity: 1, scale: 1, y: 0 }}><button className="patient-dialog-close" aria-label="Close" onClick={close}><X/></button><span className="dialog-icon"><FileHeart/></span><h2 id="patient-dialog-title">{title}</h2><p id="patient-dialog-description">{description}</p>{children}</motion.div></div>;
}

function deduplicate(requests: AccessRequest[]) {
  return [...new Map(requests.map(request => [request.ID, request])).values()];
}

function formatDate(value: string) {
  const date = new Date(value);
  const today = new Date();
  const sameDay = date.toDateString() === today.toDateString();
  return `${sameDay ? "Today" : new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(date)} at ${new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(date)}`;
}
