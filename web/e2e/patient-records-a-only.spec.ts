import { expect, test } from "@playwright/test";

test("patient record library shows a genuine Hospital A record without a fake Hospital B record", async ({ page }) => {
  const suffix = Date.now().toString(36), network = `records-network-${suffix}`, localA = `records-a-${suffix}`, localB = `records-b-${suffix}`;
  const profile = { NetworkPatientID: network, FullName: `Records Patient ${suffix}`, DateOfBirth: "1984-08-12", Gender: "Male", Phone: "555-0101" };
  expect((await page.request.post("http://127.0.0.1:8081/patients", { data: { ...profile, ID: localA } })).ok()).toBe(true);
  expect((await page.request.post("http://127.0.0.1:8082/patients", { data: { ...profile, ID: localB } })).ok()).toBe(true);
  expect((await page.request.post("http://127.0.0.1:8081/clinical-records", { headers: { "Idempotency-Key": `records-${suffix}` }, data: { ID: `records-record-${suffix}`, PatientID: localA, AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: "Source consultation", Diagnosis: "Essential hypertension", Prescription: "Lisinopril 10 mg daily", CreatedAt: new Date().toISOString() } })).ok()).toBe(true);
  await page.goto("/app");
  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByTestId("patient-switcher").selectOption(localB);
  await page.getByRole("link", { name: "My Records" }).click();

  const records = page.locator(".patient-records");
  await expect(records.getByRole("heading", { name: "My Records" })).toHaveCount(1);
  await expect(records).toContainText("Your health records across your care network");
  const hospitalA = records.locator('[data-hospital="a"]');
  const hospitalB = records.locator('[data-hospital="b"]');
  await expect(hospitalA.locator(".patient-record-card")).toHaveCount(1);
  await expect(hospitalA).toContainText("Essential hypertension");
  await expect(hospitalA).toContainText("Original record remains at Hospital A");
  await expect(hospitalA).toContainText("Integrity protected · Verified");
  await expect(hospitalB.locator(".patient-record-card")).toHaveCount(0);
  await expect(hospitalB).toContainText("No Hospital B records yet");
  await expect(records).not.toContainText("Verified referral context");

  await hospitalA.getByRole("button", { name: "View Record" }).click();
  const detail = page.getByTestId("patient-record-detail");
  await expect(detail).toContainText("Clinical Record");
  await expect(detail).toContainText("Hospital A");
  await expect(detail).toContainText("Dr. A");
  await expect(detail).toContainText("Source consultation");
  await expect(detail).toContainText("Lisinopril 10 mg daily");
  await expect(detail).toContainText("Integrity protected · Verified");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(detail.evaluate(element => element.scrollWidth <= element.clientWidth)).resolves.toBe(true);
});
