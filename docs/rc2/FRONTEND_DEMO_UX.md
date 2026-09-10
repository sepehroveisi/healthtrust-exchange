# RC2 Frontend Demonstration UX

Phase 9 adds an isolated `/authority` presentation for the deterministic RC2 professional-authority scenario. It reuses the existing Vinext, React, TypeScript, ESLint, and Playwright frontend and does not change the RC1 healthcare workflows.

## Audience modes

Operations mode explains the event and the independent Hospital A, Payer B, and Staffing Agency C responses in plain operational language. Technical mode presents the configured three-validator topology, confirmed application submissions, transaction and block references, canonical commitment comparisons, and the controlled integrity demonstration. The mode selector is presentation-only; it is not authentication, authorization, or production RBAC.

All dynamic event, response, action outcome, ledger, and verification values come from the RC2 HTTP API at `NEXT_PUBLIC_RC2_API_URL` (default `http://localhost:8090`). The frontend consumes the backend `actionPresentation` value and falls back to neutral wording when it is absent. The browser does not contact Besu RPC endpoints.

## Run locally

Start the documented RC2 Docker environment, deploy the local contracts, and start the demo API. Enable `HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED=true` only when the controlled integrity demonstration is required. Then run the frontend with Node 22 and open:

```text
http://localhost:3000/authority
```

The backend CORS origin and the browser origin must match. The default supported browser origin is `http://localhost:3000`.

## Integrity demonstration

The UI can submit only the fixed `{ "target": "HOSPITAL_A_POLICY" }` request after an explicit confirmation. The backend changes one synthetic Hospital A policy value in PostgreSQL; it does not mutate or repair the ledger. The frontend then reloads every authority, response, history, and verification view from the API.

Expected result:

- Hospital A changes from `VERIFIED` to `FAILED` because its current canonical policy evidence no longer matches the recorded commitment.
- Payer B and Staffing Agency C remain `VERIFIED`.
- Recorded ledger evidence remains unchanged.
- The failed state remains after browser refresh and API restart because it is durable database state.

Reset is intentionally not exposed through the browser. Recreate the disposable RC2 demo environment using the existing development reset procedure when a clean `VERIFIED` scenario is needed.

## Semantics and non-claims

- `VERIFIED` means current canonical evidence matches the evidence previously recorded.
- `FAILED` means a current record differs from previously recorded evidence. It does not prove which value is true and does not mean the system prevented the change.
- `INDETERMINATE` means verification could not be completed. It is not rendered as a mismatch.
- The topology diagram describes configured validators. It is not live health, peer-count, consensus-performance, throughput, or TPS telemetry.
- Response evidence is shown as the available version-level and policy commitments. The UI does not claim that a complete aggregate response object is stored on-chain.
- This is synthetic demonstration data and is not for clinical, credentialing, reimbursement, employment, or compliance decisions.

## Verification

Frontend validation includes lint, TypeScript checking, source-boundary regression tests, production build, and a real Playwright hero test. The hero test uses the Docker-backed PostgreSQL API and three-validator Besu network, confirms no mutation request occurs before the dialog confirmation, verifies the fixed request body, reloads authoritative data, and checks durable Hospital A-only failure isolation.
