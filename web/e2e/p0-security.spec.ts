import { expect, test, type APIRequestContext } from "@playwright/test";

type Scenario = { localA: string; localB: string; network: string; record: string; request: string; consent: string; diagnosis: string; name: string };

async function approvedWalkIn(request: APIRequestContext, label: string, suffix: string): Promise<Scenario> {
  const localA = `${label}-a-${suffix}`, localB = `${label}-b-${suffix}`, network = `${label}-network-${suffix}`;
  const record = `${label}-record-${suffix}`, accessRequest = `${label}-request-${suffix}`, consent = `${label}-consent-${suffix}`;
  const diagnosis = `Synthetic ${label} diagnosis ${suffix}`, name = `Synthetic ${label} Patient ${suffix}`;
  const profile = { NetworkPatientID: network, FullName: name, DateOfBirth: "1988-05-14", Gender: "Female", Phone: "555-0411" };
  expect((await request.post("http://127.0.0.1:8081/patients", { data: { ...profile, ID: localA } })).ok()).toBeTruthy();
  expect((await request.post("http://127.0.0.1:8081/clinical-records", { headers: { "Idempotency-Key": `record-${record}` }, data: { ID: record, PatientID: localA, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: `Synthetic ${label} encounter`, Diagnosis: diagnosis, Prescription: "Synthetic care plan", CreatedAt: new Date().toISOString() } })).ok()).toBeTruthy();
  expect((await request.post("http://127.0.0.1:8082/patients", { data: { ...profile, ID: localB } })).ok()).toBeTruthy();
  expect((await request.post("http://127.0.0.1:8082/visits/check-in", { data: { ID: `${label}-visit-${suffix}`, PatientID: localB, DoctorID: "doctor-b", Room: "206", Reason: "Synthetic security regression", Source: "Normal" } })).ok()).toBeTruthy();
  const discovery = await (await request.get(`http://127.0.0.1:8082/external-records/discover?patientId=${localB}&sourceOrganizationId=hospital-a`)).json();
  expect(discovery).toMatchObject({ RecordsAvailable: true, RecordCount: 1 });
  expect((await request.post("http://127.0.0.1:8082/outgoing-access-requests", { data: { ID: accessRequest, ReferralID: "", PatientID: localB, NetworkPatientID: network, RecordID: "", DiscoveryScopeID: discovery.Scopes[0].ID, RequesterActorID: "doctor-b", RequesterOrganizationID: "hospital-b", SourceOrganizationID: "hospital-a", Purpose: "Continuity of care", CreatedAt: new Date().toISOString() } })).ok()).toBeTruthy();
  expect((await request.post(`http://127.0.0.1:8081/consents/${accessRequest}/grant`, { headers: { "Idempotency-Key": `grant-${consent}` }, data: { ConsentID: consent, PatientID: localA, GrantedAt: new Date().toISOString() } })).ok()).toBeTruthy();
  return { localA, localB, network, record, request: accessRequest, consent, diagnosis, name };
}

async function openDoctorPatient(page: import("@playwright/test").Page, scenario: Scenario) {
  await page.goto("/app/patients");
  await page.getByTestId("role-switcher").selectOption("doctorB");
  const row = page.locator("article").filter({ hasText: scenario.name });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "View Patient" }).click();
}

test("external content is bound to patient, request, record and cleared on failed replacement", async ({ page, request }) => {
  const suffix = Date.now().toString(36);
  const patientA = await approvedWalkIn(request, "alpha", suffix);
  const patientB = await approvedWalkIn(request, "beta", suffix);

  await openDoctorPatient(page, patientA);
  await page.getByTestId("open-external-record").click();
  await expect(page.getByTestId("external-record")).toContainText(patientA.diagnosis);

  await page.getByRole("button", { name: "My Patients" }).click();
  await page.locator("article").filter({ hasText: patientB.name }).getByRole("button", { name: "View Patient" }).click();
  await expect(page.getByTestId("external-record")).toHaveCount(0);
  await expect(page.getByText(patientA.diagnosis)).toHaveCount(0);

  await page.route(`**/shared-records/${patientB.record}/retrieve`, route => route.abort());
  await page.getByTestId("open-external-record").click();
  await expect(page.getByTestId("external-record")).toHaveCount(0);
  await expect(page.getByText(patientA.diagnosis)).toHaveCount(0);
  await page.unroute(`**/shared-records/${patientB.record}/retrieve`);
});

test("revocation removes displayed content through polling and source retrieval stays denied", async ({ page, request, context }) => {
  const suffix = Date.now().toString(36);
  const patient = await approvedWalkIn(request, "revoke", suffix);

  await openDoctorPatient(page, patient);
  await page.getByTestId("open-external-record").click();
  await expect(page.getByTestId("external-record")).toContainText(patient.diagnosis);

  const patientPage = await context.newPage();
  await patientPage.goto("/app/requests");
  await patientPage.getByTestId("role-switcher").selectOption("patient");
  await patientPage.goto("/app/requests");
  await patientPage.getByTestId("patient-switcher").selectOption(patient.localB);
  await expect(patientPage.getByRole("button", { name: "Revoke Access" })).toBeVisible();
  await patientPage.getByRole("button", { name: "Revoke Access" }).click();
  await patientPage.getByTestId("confirm-revoke-access").click();
  await expect(patientPage.getByText("Access revoked", { exact: true }).first()).toBeVisible();

  await expect(page.getByTestId("external-record")).toHaveCount(0, { timeout: 8_000 });
  await expect(page.getByText("Access revoked", { exact: true })).toBeVisible();
  await expect(page.getByTestId("open-external-record")).toHaveCount(0);

  const denied = await request.post(`http://127.0.0.1:8082/shared-records/${patient.record}/retrieve`, { data: { ActorID: "doctor-b", OrganizationID: "hospital-b", SourceNodeID: "hospital-a-node", ReferralID: "", AccessRequestID: patient.request } });
  expect(denied.status()).toBe(403);
  await patientPage.close();
});
