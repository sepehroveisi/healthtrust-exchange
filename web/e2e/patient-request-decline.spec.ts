import { expect, test } from "@playwright/test";

test("patient reviews and declines a separate real access request", async ({ page }) => {
  test.setTimeout(90_000);
  const suffix = Date.now().toString(36), network = `decline-network-${suffix}`, localA = `decline-a-${suffix}`, localB = `decline-patient-${suffix}`, name = `Decline Patient ${suffix}`;
  expect((await page.request.post("http://127.0.0.1:8081/patients", { data: { ID: localA, NetworkPatientID: network, FullName: name, DateOfBirth: "1984-08-12", Gender: "Male", Phone: "555-0101" } })).ok()).toBe(true);
  await page.goto("/app");
  await page.getByTestId("role-switcher").selectOption("doctorA");
  await page.getByPlaceholder("Search patients by name, ID or phone...").fill(name);
  await page.locator(".patient-directory-list article").filter({ hasText: name }).getByRole("button", { name: "View Patient" }).click();
  await page.getByTestId("source-visit").getByRole("button", { name: "Complete Visit" }).click();
  await page.getByTestId("create-referral").click();

  await page.getByTestId("role-switcher").selectOption("reception");
  await page.getByRole("button", { name: "Check In Patient" }).first().click();
  const checkIn = page.getByRole("dialog", { name: "Check In Patient" });
  await checkIn.getByRole("button", { name: "Register New Patient" }).click();
  const registration = checkIn.getByTestId("patient-registration");
  await registration.getByLabel("Full name").fill(name);
  await registration.getByLabel("Date of birth").fill("1984-08-12");
  await registration.getByLabel("Male", { exact: true }).check();
  await registration.getByLabel("Phone").fill("555-0101");
  await registration.getByLabel("Existing care network ID").fill(network);
  await registration.getByRole("button", { name: "Register and Continue" }).click();
  await checkIn.getByRole("button", { name: "Use Referral" }).click();
  await checkIn.getByLabel("Room").selectOption("203");
  await checkIn.getByLabel("Visit reason").selectOption("Post-treatment follow-up");
  await checkIn.getByRole("button", { name: "Check In Patient" }).click();

  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.getByRole("link", { name: "My Patients" }).click();
  const directory = page.locator(".patient-directory");
  await directory.getByPlaceholder("Search patients by name, ID or phone...").fill(name);
  await directory.locator("article").filter({ hasText: name }).getByRole("button", { name: "View Patient" }).click();
  await page.locator(".doctor-patient-detail").getByTestId("request-access").click();

  await page.getByTestId("role-switcher").selectOption("patient");
  await page.getByTestId("patient-switcher").selectOption(localB);
  await page.getByRole("link", { name: "Requests" }).click();
  const trigger = page.getByRole("button", { name: "Review Request" });
  await expect(page.getByRole("heading", { name: "Needs Your Attention" })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await trigger.click();
  const review = page.getByRole("dialog", { name: "Dr. B at Hospital B is requesting access" });
  await expect(review.evaluate(element => element.scrollWidth <= element.clientWidth)).resolves.toBe(true);
  await expect(review.getByRole("button", { name: "Allow Access" })).toBeVisible();
  await expect(review.getByRole("button", { name: "Decline", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(review).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await review.getByRole("button", { name: "Decline", exact: true }).click();
  await expect(page.getByText("Request declined", { exact: true }).first()).toBeVisible();
  await expect(page.locator(".past-request-list")).toContainText("Request declined");
  await expect(page.locator(".patient-request-empty").filter({ hasText: "No active external access" })).toBeVisible();
  await page.reload();
  await expect(page.locator(".past-request-list")).toContainText("Request declined");
  await expect(page.getByRole("button", { name: "Review Request" })).toHaveCount(0);

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByTestId("role-switcher").selectOption("doctorB");
  await page.getByRole("link", { name: "My Patients" }).click();
  const refreshedDirectory = page.locator(".patient-directory");
  await refreshedDirectory.getByPlaceholder("Search patients by name, ID or phone...").fill(name);
  await refreshedDirectory.locator("article").filter({ hasText: name }).getByRole("button", { name: "View Patient" }).click();
  await expect(page.locator(".doctor-patient-detail")).toContainText("Access declined");
});
