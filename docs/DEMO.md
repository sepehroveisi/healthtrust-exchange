# HealthTrust Exchange — 5-minute demo

## 00:00 — Problem

Clinical information is commonly split across organizations. Introduce Patient P, whose record is held by Hospital A but whose referred care takes place at Hospital B.

## 00:30 — Architecture

Show the two independent hospital nodes and databases. Explain that clinical content stays in PostgreSQL while both nodes validate a shared permissioned audit chain.

## 01:00 — Doctor A creates a record

Open Demo Mode as Doctor A. Create the clinical record. Point out that the backend first persists the content off-chain, then confirms its integrity commitment.

## 01:30 — Referral

Create the Hospital A → Hospital B referral. Show the real `ReferralCreated` event.

## 02:00 — Doctor B is denied

Switch to Doctor B, request access, and attempt retrieval. The source hospital denies access because no patient consent exists.

## 02:20 — Patient consent

Switch to Patient P. Review the requesting doctor, organization, record, and timestamp. Grant access.

## 02:50 — Doctor B accesses the record

Return to Doctor B and open the record. Hospital B generates the requester proof and peer envelope server-side; Hospital A independently validates identity, peer, consent, and replay state.

## 03:20 — Integrity verification

Select **Verify integrity**. The source hospital recomputes the canonical record hash and compares it with the blockchain commitment.

## 03:40 — Audit timeline

Show `RecordCommitted`, `ReferralCreated`, `ConsentGranted`, and `RecordAccessed`, with real actors, organizations, times, and block heights.

## 04:00 — Revocation

As Patient P, revoke consent. Return to Doctor B and demonstrate that retrieval is denied again. Show `ConsentRevoked` in the audit timeline.

## 04:20 — Engineering detail

Highlight Go, two PostgreSQL databases, Ed25519 signatures, durable replay prevention, idempotency, recovery, reconciliation, Next.js, and real Playwright E2E.

## 04:50 — Closing

HealthTrust Exchange is an engineering prototype: a concrete exploration of patient-controlled, cross-organization exchange with off-chain clinical data and shared integrity evidence—not a production EHR or compliance-certified system.
