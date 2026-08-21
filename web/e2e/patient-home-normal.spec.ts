import { expect, test } from "@playwright/test";

test("patient home follows a normal Hospital B visit from check-in to record", async ({ page }) => {
  test.setTimeout(90_000);
  const suffix = Date.now().toString(36), name = `Normal Patient ${suffix}`;
  await page.goto("/app");

  await page.getByRole("button", { name: "Check In Patient" }).first().click();
  const modal = page.getByRole("dialog", { name: "Check In Patient" });
  await modal.getByRole("button", { name: "Register New Patient" }).click();
  const registration = modal.getByTestId("patient-registration");
  await registration.getByLabel("Full name").fill(name);
  await registration.getByLabel("Date of birth").fill("1984-08-12");
  await registration.getByLabel("Male", { exact: true }).check();
  await registration.getByLabel("Phone").fill("555-0101");
  await registration.getByRole("button", { name: "Register and Continue" }).click();
  await modal.getByLabel("Assigned doctor").selectOption("doctor-b");
  await modal.getByLabel("Room").selectOption("203");
  await modal.getByLabel("Visit reason").selectOption("General consultation");
  await modal.getByRole("button", { name: "Check In Patient" }).click();

  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByTestId("patient-switcher").selectOption(`normal-patient-${suffix}`);
  const home = page.locator(".patient-home");
  await expect(home.getByTestId("patient-care-journey")).toHaveAttribute("data-kind", "normal");
  await expect(home.locator('[aria-current="step"]')).toContainText("Waiting for your doctor");
  await expect(home.locator(".patient-action-required")).toHaveCount(0);

  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.getByRole("link", { name: "My Patients" }).click();
  const directory = page.locator(".patient-directory");
  await directory.getByPlaceholder("Search patients by name, ID or phone...").fill(name);
  await directory.locator("article").filter({ hasText: name }).getByRole("button", { name: "View Patient" }).click();
  await page.getByTestId("start-visit").click();
  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByRole("link", { name: "Home", exact: true }).click();
  await expect(page.locator('[aria-current="step"]')).toContainText("Visit in progress");

  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.getByRole("link", { name: "My Patients" }).click();
  const activeDirectory = page.locator(".patient-directory");
  await activeDirectory.getByPlaceholder("Search patients by name, ID or phone...").fill(name);
  await activeDirectory.locator("article").filter({ hasText: name }).getByRole("button", { name: "View Patient" }).click();
  await page.locator(".doctor-patient-detail").getByRole("button", { name: "Continue Visit" }).click();
  const documentation = page.getByTestId("visit-documentation");
  await documentation.getByLabel("Clinical notes").fill("Synthetic normal-visit test note");
  await documentation.getByLabel("Diagnosis").fill("Routine examination");
  await documentation.getByLabel("Treatment / Plan").fill("Follow up as needed");
  await documentation.getByTestId("complete-visit").click();
  await page.getByRole("dialog", { name: "Complete this visit?" }).getByRole("button", { name: "Complete Visit" }).click();

  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByRole("link", { name: "Home", exact: true }).click();
  await expect(page.locator('[aria-current="step"]')).toContainText("Record available");
  await expect(page.locator(".patient-recent")).toContainText("Hospital B record available");
  await expect(page.locator(".patient-ownership-note")).toHaveCount(0);
  await page.getByRole("link", { name: "My Records" }).click();
  const records = page.locator(".patient-records");
  await expect(records.locator('[data-hospital="a"] .patient-record-card')).toHaveCount(0);
  await expect(records.locator('[data-hospital="b"] .patient-record-card')).toHaveCount(1);
  await expect(records.locator('[data-hospital="b"]')).toContainText("Routine examination");
  await expect(records.locator('[data-hospital="b"]')).toContainText("Integrity protected · Verified");
  await records.locator('[data-hospital="b"]').getByRole("button", { name: "View Record" }).click();
  await expect(page.getByTestId("patient-record-detail")).toContainText("Synthetic normal-visit test note");
  await page.getByRole("button", { name: "Back to My Records" }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".patient-records").evaluate(element => element.scrollWidth <= element.clientWidth)).resolves.toBe(true);
});
