import { expect, test } from "@playwright/test";

test("walk-in patient uses discovery, scoped consent, secure viewing, and revocation with no referral", async ({ page, request }) => {
  const suffix = Date.now().toString(36);
  const localA = `jean-a-${suffix}`, localB = `jean-b-${suffix}`, network = `network-jean-${suffix}`;
  const recordA = `record-a-${suffix}`, visit = `visit-b-${suffix}`;
  const now = new Date().toISOString();
  const patient = { NetworkPatientID: network, FullName: `Jean Grey ${suffix}`, DateOfBirth: "1986-04-18", Gender: "Female", Phone: "555-0299" };

  expect((await request.post("http://127.0.0.1:8081/patients", { data: { ...patient, ID: localA } })).ok()).toBeTruthy();
  expect((await request.post("http://127.0.0.1:8081/clinical-records", { headers: { "Idempotency-Key": `record-${suffix}` }, data: { ID: recordA, PatientID: localA, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: "Synthetic source consultation", Diagnosis: "Synthetic migraine history", Prescription: "Synthetic care plan", CreatedAt: now } })).ok()).toBeTruthy();
  expect((await request.post("http://127.0.0.1:8082/patients", { data: { ...patient, ID: localB } })).ok()).toBeTruthy();
  expect((await request.post("http://127.0.0.1:8082/visits/check-in", { data: { ID: visit, PatientID: localB, DoctorID: "doctor-b", Room: "204", Reason: "Walk-in consultation", Source: "Normal" } })).ok()).toBeTruthy();
  expect((await request.post(`http://127.0.0.1:8082/visits/${visit}/start`, { data: { DoctorID: "doctor-b" } })).ok()).toBeTruthy();
  expect(await (await request.get(`http://127.0.0.1:8081/referrals?patientId=${localA}`)).json()).toEqual([]);
  expect(await (await request.get(`http://127.0.0.1:8082/incoming-referrals?patientId=${localB}`)).json()).toEqual([]);

  await page.goto("/app/patients");
  await page.getByTestId("role-switcher").selectOption("doctorB");
  const row = page.locator("article").filter({ hasText: patient.FullName });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "View Patient" }).click();
  const external = page.getByTestId("external-records");
  await expect(external).toContainText("Previous records are available");
  await expect(external).not.toContainText("Synthetic migraine history");
  await external.getByTestId("request-access").click();
  await expect(external).toContainText("Approval pending");

  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByTestId("patient-switcher").selectOption(localB);
  await page.goto("/app/requests");
  await page.getByRole("button", { name: "Review Request" }).click();
  await expect(page.getByRole("dialog")).not.toContainText("referral");
  await page.getByTestId("allow-access").click();
  await expect(page.getByText("Access approved", { exact: true }).first()).toBeVisible();

  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.goto("/app/patients");
  await page.locator("article").filter({ hasText: patient.FullName }).getByRole("button", { name: "View Patient" }).click();
  await page.getByTestId("open-external-record").click();
  await expect(page.getByTestId("external-record")).toContainText("Synthetic migraine history");
  await expect(page.getByTestId("external-record")).toContainText("remains stored at Hospital A");

  await page.getByRole("button", { name: "Continue Visit" }).click();
  const form = page.getByTestId("visit-documentation");
  await form.getByLabel("Clinical notes").fill("Walk-in examination completed");
  await form.getByLabel("Diagnosis").fill("Synthetic follow-up diagnosis");
  await form.getByRole("button", { name: "Complete Visit" }).click();
  await page.getByRole("dialog", { name: "Complete this visit?" }).getByRole("button", { name: "Complete Visit" }).click();
  await expect(page.getByTestId("local-record")).toContainText("Synthetic follow-up diagnosis");

  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByTestId("patient-switcher").selectOption(localB);
  await page.goto("/app/records");
  await expect(page.locator('[data-hospital="a"]')).toContainText("Synthetic migraine history");
  await expect(page.locator('[data-hospital="b"]')).toContainText("Synthetic follow-up diagnosis");

  await page.goto("/app/requests");
  await page.getByRole("button", { name: "Revoke Access" }).click();
  await page.getByTestId("confirm-revoke-access").click();
  await expect(page.getByText("Access revoked", { exact: true }).first()).toBeVisible();
  const outgoing = await (await request.get(`http://127.0.0.1:8082/outgoing-access-requests?patientId=${localB}`)).json();
  const denied = await request.post(`http://127.0.0.1:8082/shared-records/${recordA}/retrieve`, { data: { ActorID: "doctor-b", OrganizationID: "hospital-b", SourceNodeID: "hospital-a-node", ReferralID: "", AccessRequestID: outgoing[0].Request.ID } });
  expect(denied.status()).toBe(403);
});

test("cross-patient discovery does not leak availability", async ({ request }) => {
  const suffix = Date.now().toString(36);
  const jeanNetwork = `iso-jean-${suffix}`, bruceNetwork = `iso-bruce-${suffix}`;
  await request.post("http://127.0.0.1:8081/patients", { data: { ID: `iso-a-${suffix}`, NetworkPatientID: jeanNetwork, FullName: "Jean Grey", DateOfBirth: "1986-04-18", Gender: "Female", Phone: "555-0301" } });
  await request.post("http://127.0.0.1:8081/clinical-records", { headers: { "Idempotency-Key": `iso-${suffix}` }, data: { ID: `iso-record-${suffix}`, PatientID: `iso-a-${suffix}`, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: "Synthetic", Diagnosis: "Synthetic", Prescription: "Synthetic", CreatedAt: new Date().toISOString() } });
  await request.post("http://127.0.0.1:8082/patients", { data: { ID: `iso-jean-b-${suffix}`, NetworkPatientID: jeanNetwork, FullName: "Jean Grey", DateOfBirth: "1986-04-18", Gender: "Female", Phone: "555-0301" } });
  await request.post("http://127.0.0.1:8082/patients", { data: { ID: `iso-bruce-b-${suffix}`, NetworkPatientID: bruceNetwork, FullName: "Bruce Wayne", DateOfBirth: "1979-02-19", Gender: "Male", Phone: "555-0302" } });
  const jean = await (await request.get(`http://127.0.0.1:8082/external-records/discover?patientId=iso-jean-b-${suffix}&sourceOrganizationId=hospital-a`)).json();
  const bruce = await (await request.get(`http://127.0.0.1:8082/external-records/discover?patientId=iso-bruce-b-${suffix}&sourceOrganizationId=hospital-a`)).json();
  expect(jean).toMatchObject({ RecordsAvailable: true, RecordCount: 1 });
  expect(bruce).toMatchObject({ RecordsAvailable: false, RecordCount: 0 });
});

test("referral path remains independently supported", async ({ page, request }) => {
  const suffix = Date.now().toString(36), patientID = `ref-patient-${suffix}`, recordID = `ref-record-${suffix}`, referralID = `referral-${suffix}`;
  const profile = { ID: patientID, NetworkPatientID: `ref-network-${suffix}`, FullName: `Referral Patient ${suffix}`, DateOfBirth: "1984-08-12", Gender: "Male", Phone: "555-0399" };
  await request.post("http://127.0.0.1:8081/patients", { data: profile });
  await request.post("http://127.0.0.1:8081/clinical-records", { headers: { "Idempotency-Key": `ref-record-${suffix}` }, data: { ID: recordID, PatientID: patientID, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: "Synthetic referred consultation", Diagnosis: "Synthetic referred diagnosis", Prescription: "Synthetic plan", CreatedAt: new Date().toISOString() } });
  expect((await request.post("http://127.0.0.1:8081/referrals", { headers: { "Idempotency-Key": `referral-${suffix}` }, data: { ID: referralID, PatientID: patientID, RecordID: recordID, SourceOrganizationID: "hospital-a", DestinationOrganizationID: "hospital-b", CreatedByActorID: "doctor-a", CreatedAt: new Date().toISOString() } })).ok()).toBeTruthy();
  await request.post("http://127.0.0.1:8082/patients", { data: profile });
  await request.post("http://127.0.0.1:8082/visits/check-in", { data: { ID: `ref-visit-${suffix}`, PatientID: patientID, DoctorID: "doctor-b", Room: "203", Reason: "Referred follow-up", ReferralID: referralID, Source: "Referral" } });

  await page.goto("/app/patients");
  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.locator("article").filter({ hasText: profile.FullName }).getByRole("button", { name: "View Patient" }).click();
  const external = page.getByTestId("external-records");
  await expect(external).toContainText("External record access requires patient approval");
  await external.getByTestId("request-access").click();
  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByTestId("patient-switcher").selectOption(patientID);
  await page.goto("/app/requests");
  await page.getByRole("button", { name: "Review Request" }).click();
  await page.getByTestId("allow-access").click();
  await expect(page.getByText("Access approved", { exact: true }).first()).toBeVisible();
  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.goto("/app/patients");
  await page.locator("article").filter({ hasText: profile.FullName }).getByRole("button", { name: "View Patient" }).click();
  await expect(page.getByTestId("external-records")).toContainText("Available securely");
  await page.getByTestId("open-external-record").click();
  await expect(page.getByTestId("external-record")).toContainText("Synthetic referred diagnosis");
});
