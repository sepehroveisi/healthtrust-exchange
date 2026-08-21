import { execFileSync } from "node:child_process";
import { expect, test } from "@playwright/test";

test("stale operational visit does not block today's visit and remains historical", async ({ page, request }) => {
  const suffix = Date.now().toString(36), staleID = `stale-${suffix}`, currentID = `current-${suffix}`;
  for (const patient of [{ ID: staleID, FullName: `Stale Patient ${suffix}` }, { ID: currentID, FullName: `Current Patient ${suffix}` }]) {
    expect((await request.post("http://127.0.0.1:8082/patients", { data: { ...patient, NetworkPatientID: patient.ID, DateOfBirth: "1980-01-01", Gender: "Other", Phone: "555-0500" } })).ok()).toBe(true);
  }
  const staleVisit = `stale-visit-${suffix}`, currentVisit = `current-visit-${suffix}`;
  expect((await request.post("http://127.0.0.1:8082/visits/check-in", { data: { ID: staleVisit, PatientID: staleID, DoctorID: "doctor-b", Room: "205", Reason: "Old unfinished visit", Source: "Normal" } })).ok()).toBe(true);
  const yesterday = String((Date.now() - 86_400_000) * 1_000_000);
  execFileSync("docker", ["compose", "-f", "../docker-compose.yml", "exec", "-T", "postgres-b", "psql", "-U", "healthtrust", "-d", "healthtrust_b", "-c", `UPDATE visits SET checked_in_at_unix_nano=${yesterday}, status='InVisit', started_at_unix_nano=${yesterday} WHERE id='${staleVisit}'`]);
  expect((await request.post("http://127.0.0.1:8082/visits/check-in", { data: { ID: currentVisit, PatientID: currentID, DoctorID: "doctor-b", Room: "204", Reason: "Today's legitimate visit", Source: "Normal" } })).ok()).toBe(true);
  expect((await request.post(`http://127.0.0.1:8082/visits/${currentVisit}/start`, { data: { DoctorID: "doctor-b" } })).ok()).toBe(true);
  await page.goto("/app");
  await page.getByTestId("role-switcher").selectOption("doctorB");
  await expect(page.locator(".doctor-primary")).toContainText(`Current Patient ${suffix}`);
  await page.getByRole("link", { name: "My Patients" }).click();
  await page.getByRole("button", { name: "All" }).click();
  await page.getByPlaceholder("Search patients by name, ID or phone...").fill(`Stale Patient ${suffix}`);
  await expect(page.locator(".patient-directory-list")).toContainText(`Stale Patient ${suffix}`);
  expect((await request.put(`http://127.0.0.1:8082/visits/${currentVisit}/draft`, { data: { DoctorID: "doctor-b", Draft: { ChiefComplaint: "Current", ClinicalNotes: "Completed synthetic current visit", Diagnosis: "Stable", Prescription: "", FollowUpPlan: "None" } } })).ok()).toBe(true);
  expect((await request.post(`http://127.0.0.1:8082/visits/${currentVisit}/complete`, { data: { DoctorID: "doctor-b", RecordID: `stale-recovery-record-${suffix}`, Draft: { ChiefComplaint: "Current", ClinicalNotes: "Completed synthetic current visit", Diagnosis: "Stable", Prescription: "", FollowUpPlan: "None" } } })).ok()).toBe(true);
});

test("discovery failure is distinct from zero results and retry recovers", async ({ page, request }) => {
  const suffix = Date.now().toString(36), network = `failure-network-${suffix}`, localA = `failure-a-${suffix}`, localB = `failure-b-${suffix}`, name = `Discovery Patient ${suffix}`;
  const profile = { NetworkPatientID: network, FullName: name, DateOfBirth: "1985-05-05", Gender: "Female", Phone: "555-0600" };
  expect((await request.post("http://127.0.0.1:8081/patients", { data: { ...profile, ID: localA } })).ok()).toBe(true);
  expect((await request.post("http://127.0.0.1:8082/patients", { data: { ...profile, ID: localB } })).ok()).toBe(true);
  expect((await request.post("http://127.0.0.1:8081/clinical-records", { headers: { "Idempotency-Key": `failure-${suffix}` }, data: { ID: `failure-record-${suffix}`, PatientID: localA, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: "Synthetic", Diagnosis: "Synthetic", Prescription: "Synthetic", CreatedAt: new Date().toISOString() } })).ok()).toBe(true);
  await page.route("**/external-records/discover?**", route => route.abort("failed"));
  await page.goto("/app");
  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.getByRole("link", { name: "My Patients" }).click();
  await page.getByRole("button", { name: "All" }).click();
  const directory = page.locator(".patient-directory");
  await directory.getByPlaceholder("Search patients by name, ID or phone...").fill(name);
  await directory.locator("article").filter({ hasText: name }).getByRole("button", { name: "View Patient" }).click();
  const external = page.getByTestId("external-records");
  await expect(external).toContainText("We couldn't check external records right now.");
  await expect(external).not.toContainText("No external records found.");
  await page.unroute("**/external-records/discover?**");
  await external.getByRole("button", { name: "Retry" }).click();
  await expect(external).toContainText("Previous records are available for this patient.");
});
