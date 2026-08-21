"use client";
/* eslint-disable @next/next/no-html-link-for-pages -- vinext navigation uses the app's established full-page anchors */

import { motion, useReducedMotion } from "framer-motion";
import { ArrowRight, Bell, Check, Circle, FileHeart, MapPin, ShieldCheck } from "lucide-react";
import type { Patient, Visit } from "../lib/api/operations-workflow";
import type { ClinicalRecord } from "../lib/api/clinical";
import type { Referral } from "../lib/api/referrals";
import type { AccessRequest } from "../lib/api/access";
import type { Consent } from "../lib/api/consent";
import type { PatientJourney } from "../lib/api/journey";

type Stage = { label: string; description: string; status: "complete" | "current" | "future" };
type Activity = { label: string; detail: string; at: string; trusted?: boolean };

type PatientHomeProps = {
  patientID: string;
  journeyOnly?: boolean;
  patients: Patient[];
  visits: Visit[];
  recordsA: ClinicalRecord[];
  recordsB: ClinicalRecord[];
  referrals: Referral[];
  requests: AccessRequest[];
  outgoing: AccessRequest[];
  consents: Consent[];
  journey: PatientJourney | null;
};

const newest = <T extends { CreatedAt?: string }>(values: T[]) => [...values].sort((a, b) => new Date(b.CreatedAt ?? 0).valueOf() - new Date(a.CreatedAt ?? 0).valueOf())[0];
const stageStatus = (index: number, current: number): Stage["status"] => index < current ? "complete" : index === current ? "current" : "future";
const displayTime = (value: string) => new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));

export function PatientHome(props: PatientHomeProps) {
  const patientID = props.patientID;
  const patient = props.patients.find(value => value.ID === patientID);
  const visits = props.visits.filter(value => value.PatientID === patientID).sort((a, b) => new Date(b.CheckedInAt).valueOf() - new Date(a.CheckedInAt).valueOf());
  const visit = visits[0];
  const referral = newest(props.referrals.filter(value => value.PatientID === patientID));
  const request = newest([...props.requests.filter(value => value.NetworkPatientID === patient?.NetworkPatientID || value.PatientID === patientID), ...props.outgoing.filter(value => value.PatientID === patientID)]);
  const consent = [...props.consents].filter(value => value.AccessRequestID === request?.ID).sort((a, b) => new Date(b.GrantedAt).valueOf() - new Date(a.GrantedAt).valueOf())[0];
  const localRecords = props.recordsB.filter(value => value.PatientID === patientID);
  const referred = Boolean(referral);
  const activeConsent = consent?.Status === "Active";
  const revoked = consent?.Status === "Revoked";
  const accessed = props.journey?.Steps.some(value => value.Name === "Dr. B viewed the referred record" && value.Status === "Complete") ?? false;
  const actionable = Boolean(request && !consent && request.Status !== "Denied" && request.Status !== "Revoked");
  const stages = referred ? referredStages({ visit, request, activeConsent, revoked, accessed }) : request ? walkInStages({ visit, request, activeConsent, revoked, accessed }) : normalStages(visit, localRecords.length > 0);
  const current = stages.find(value => value.status === "current") ?? stages.at(-1)!;
  const activities = recentActivity({ visit, referral, request, consent, localRecords });
  const name = patient?.FullName || "Patient";

  if (props.journeyOnly) return <section className="patient-home journey-only"><header className="patient-greeting"><span className="eyebrow">My care</span><h1>My Care Journey</h1><p>{current.description}</p></header><CareJourney stages={stages} referred={referred}/></section>;

  return <main className="patient-home"><header className="patient-greeting"><span className="eyebrow">Your care today</span><h1>Hello, {name}</h1><p>{current.description}</p></header>
    {actionable && <motion.section className="patient-action-required" role="status" initial={{ opacity: 0, y: 5 }} animate={{ opacity: 1, y: 0 }}><span className="patient-action-icon"><Bell/></span><div><span className="eyebrow">Action required</span><h2>Dr. B is requesting access</h2><p>Dr. B at Hospital B would like to view your medical record from Hospital A for continuity of care.</p></div><a className="button" href="/app/requests">Review request <ArrowRight/></a></motion.section>}
    <section className="patient-journey-section" aria-labelledby="care-journey-title"><div className="patient-section-heading"><div><span className="eyebrow">Current care</span><h2 id="care-journey-title">My Care Journey</h2></div><a href="/app/journey">View journey <ArrowRight/></a></div><CareJourney stages={stages} referred={referred}/></section>
    <section className="patient-recent" aria-labelledby="recent-activity-title"><div className="patient-section-heading"><div><span className="eyebrow">Latest updates</span><h2 id="recent-activity-title">Recent activity</h2></div>{localRecords.length > 0 && <a href="/app/records">View records <ArrowRight/></a>}</div>{activities.length ? <div className="patient-activity-list">{activities.map((activity, index) => <motion.article key={`${activity.label}-${activity.at}`} initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * .035 }}><span className="activity-icon">{activity.trusted ? <ShieldCheck/> : <FileHeart/>}</span><div><h3>{activity.label}</h3><p>{activity.detail}</p></div><time dateTime={activity.at}>{displayTime(activity.at)}</time></motion.article>)}</div> : <div className="patient-calm-empty"><b>No recent care activity.</b><span>Your updates will appear here.</span></div>}</section>
  </main>;
}

function CareJourney({ stages, referred }: { stages: Stage[]; referred: boolean }) {
  const reduce = useReducedMotion();
  const current = Math.max(0, stages.findIndex(value => value.status === "current"));
  const progress = stages.length > 1 ? current / (stages.length - 1) : 0;
  return <div className="care-journey" data-testid="patient-care-journey" data-kind={referred ? "referred" : "normal"}><div className="journey-progress" aria-hidden="true"><motion.span initial={reduce ? false : { scaleX: 0 }} animate={{ scaleX: progress }} transition={{ duration: .55, ease: "easeOut" }}/></div><ol>{stages.map((stage, index) => <motion.li key={stage.label} className={stage.status} aria-current={stage.status === "current" ? "step" : undefined} initial={reduce ? false : { opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * .045 }}><span className="journey-node" aria-hidden="true">{stage.status === "complete" ? <Check/> : <Circle/>}</span><div><span className="stage-status">{stage.status === "complete" ? "Completed" : stage.status === "current" ? "Current step" : "What comes next"}</span><h3>{stage.label}</h3><p>{stage.description}</p></div></motion.li>)}</ol>{referred && <div className="record-location"><div><MapPin/><span><b>Hospital A</b>Your original record stays here.</span></div><ArrowRight/><div><ShieldCheck/><span><b>Hospital B</b>Secure viewing when you approve.</span></div></div>}</div>;
}

function normalStages(visit: Visit | undefined, recordAvailable: boolean): Stage[] {
  let current = 0;
  if (visit?.Status === "Waiting") current = 1;
  if (visit?.Status === "InVisit") current = 2;
  if (visit?.Status === "Completed") current = recordAvailable ? 4 : 3;
  const values = [
    ["Check in at Hospital B", "Reception will confirm your arrival."],
    ["Waiting for your doctor", "You're checked in. Dr. B will see you when ready."],
    ["Visit in progress", "Your visit with Dr. B is currently in progress."],
    ["Visit completed", "Your visit is complete. Your care team is finalizing the record."],
    ["Record available", "Your new Hospital B record is available and integrity protected."],
  ];
  return values.map(([label, description], index) => ({ label, description, status: stageStatus(index, current) }));
}

function referredStages({ visit, request, activeConsent, revoked, accessed }: { visit?: Visit; request?: AccessRequest; activeConsent: boolean; revoked: boolean; accessed: boolean }): Stage[] {
  if (revoked) {
    const completed = visit?.Status === "Completed";
    const finalLabel = completed ? "Visit completed" : visit?.Status === "InVisit" ? "Visit in progress" : "Continue your visit";
    const finalDescription = completed ? "Your visit is complete and your new Hospital B record is available." : visit?.Status === "InVisit" ? "Your visit with Dr. B is currently in progress." : "Dr. B will continue your care at Hospital B.";
    const current = 4;
    const values = [
      ["Referred to Hospital B", "Hospital A sent your referral."],
      ["Checked in", "You arrived at Hospital B."],
      ["Record access requested", "Dr. B asked to securely view your Hospital A record."],
      ["Access ended", "You revoked Dr. B's access to your Hospital A record."],
      [finalLabel, finalDescription],
    ];
    return values.map(([label, description], index) => ({ label, description, status: stageStatus(index, current) }));
  }
  let current = 1;
  if (visit) current = 2;
  if (request) current = 3;
  if (activeConsent || revoked) current = 4;
  if (accessed) current = 5;
  const completed = visit?.Status === "Completed";
  const finalLabel = completed ? "Visit completed" : visit?.Status === "InVisit" ? "Visit in progress" : "Continue your visit";
  const finalDescription = completed ? "Your visit is complete and your new Hospital B record is available." : visit?.Status === "InVisit" ? "Your visit with Dr. B is currently in progress." : "Dr. B will continue your care at Hospital B.";
  const values = [
    ["Referred to Hospital B", "Hospital A sent your referral."],
    ["Checked in", "You arrived at Hospital B."],
    ["Record access requested", "Dr. B asked to securely view your Hospital A record."],
    ["Your approval", activeConsent ? "You approved secure access for Dr. B." : "Dr. B needs your permission before viewing your record."],
    ["Record securely available", accessed ? "Dr. B securely viewed the approved record." : "Dr. B can securely view the approved Hospital A record."],
    [finalLabel, finalDescription],
  ];
  if (completed) current = 5;
  return values.map(([label, description], index) => ({ label, description, status: stageStatus(index, current) }));
}

function walkInStages({ visit, request, activeConsent, revoked, accessed }: { visit?: Visit; request?: AccessRequest; activeConsent: boolean; revoked: boolean; accessed: boolean }): Stage[] {
  let current = 1;
  if (visit?.Status === "InVisit") current = 2;
  if (request) current = 3;
  if (activeConsent || revoked) current = 4;
  if (accessed || visit?.Status === "Completed") current = 5;
  const values = [
    ["Checked in", "You arrived at Hospital B as a walk-in patient."],
    ["Waiting for your doctor", "Dr. B will see you when ready."],
    ["Visit in progress", "Your visit with Dr. B is in progress."],
    ["Record access requested", "Dr. B requested your approval to view one Hospital A record."],
    [revoked ? "Access ended" : "Your approval", revoked ? "You revoked access to the requested record." : activeConsent ? "You approved secure access for Dr. B." : "Your decision is required."],
    [visit?.Status === "Completed" ? "Visit completed" : "Secure viewing", visit?.Status === "Completed" ? "Your Hospital B visit is complete." : "The approved record can be viewed securely."],
  ];
  return values.map(([label, description], index) => ({ label, description, status: stageStatus(index, current) }));
}

function recentActivity({ visit, referral, request, consent, localRecords }: { visit?: Visit; referral?: Referral; request?: AccessRequest; consent?: Consent; localRecords: ClinicalRecord[] }): Activity[] {
  const values: Activity[] = [];
  if (referral) values.push({ label: "Referral received at Hospital B", detail: "Hospital B received your referral from Hospital A.", at: referral.CreatedAt });
  if (visit) values.push({ label: visit.Status === "Completed" ? "Visit completed at Hospital B" : visit.Status === "InVisit" ? "Visit started with Dr. B" : "Checked in at Hospital B", detail: visit.Status === "Completed" ? "Your Hospital B visit is complete." : visit.Status === "InVisit" ? "Your consultation is in progress." : "You are waiting for Dr. B.", at: visit.CompletedAt || visit.StartedAt || visit.CheckedInAt });
  if (request) values.push({ label: "Access request received", detail: "Dr. B requested access to your Hospital A record.", at: request.CreatedAt });
  if (consent) values.push({ label: consent.Status === "Revoked" ? "Access revoked" : "Access approved", detail: consent.Status === "Revoked" ? "Dr. B can no longer retrieve the requested record." : "Dr. B can securely view the approved record.", at: consent.RevokedAt || consent.GrantedAt });
  for (const record of localRecords) values.push({ label: "Hospital B record available", detail: "A new record was created by your Hospital B care team.", at: record.CreatedAt, trusted: true });
  return values.filter(value => value.at).sort((a, b) => new Date(b.at).valueOf() - new Date(a.at).valueOf()).slice(0, 5);
}
