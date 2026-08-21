# Portfolio descriptions

## One sentence

Built a two-hospital healthcare information-exchange prototype in Go, PostgreSQL, and Next.js with patient-controlled access, off-chain clinical storage, Ed25519 authentication, and blockchain-backed integrity auditing.

## Short paragraph

HealthTrust Exchange explores secure clinical-data sharing across independent healthcare organizations. I built two Go hospital nodes with separate PostgreSQL databases, a custom permissioned blockchain for integrity and audit events, patient consent and revocation, signed peer/requester proofs, replay prevention, restart recovery, and a role-based Next.js product demo verified through real Docker-backed Playwright E2E testing.

## Technical description

Designed and implemented an engineering prototype consisting of two independently persisted Go services, a custom signed permissioned blockchain, deterministic recovery and synchronization, durable idempotency and replay controls, and a secure server-side cross-hospital retrieval flow. Clinical content remains in source-hospital PostgreSQL storage; SHA-256 commitments and authorization events are recorded on-chain. A TypeScript/React/Next.js frontend exposes Doctor, Patient, and Staff workflows, with the full consent lifecycle tested against both real databases and nodes using Playwright. The project is explicitly a prototype, not a production EHR or compliance-certified system.
