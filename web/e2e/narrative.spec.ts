import { expect, test } from "@playwright/test";

async function center(page: import("@playwright/test").Page, testid: string) {
  await page.waitForLoadState("networkidle");
  await page.getByTestId(testid).getByRole("button").click();
  await page.waitForTimeout(550);
}

test("RC1 stage preserves ownership and maps authorization through denial", async ({ page }) => {
  await page.goto("/demo");
  const stage = page.locator(".rc1-stage");
  await expect(stage.getByText("Hospital A", { exact: true })).toBeVisible();
  await expect(stage.getByText("Source of record", { exact: true })).toBeVisible();
  await expect(stage.getByText("Hospital B", { exact: true })).toBeVisible();
  await expect(stage.getByText("Patient P", { exact: true })).toBeVisible();
  await expect(stage.getByText("Owned by Hospital A", { exact: true })).toBeVisible();
  await expect(stage.getByText("Stored off-chain", { exact: true })).toBeVisible();

  await center(page, "story-authorization");
  await expect(stage).toHaveAttribute("data-step", "3");
  await expect(page.getByTestId("story-authorization")).toHaveAttribute("data-active", "true");
  await expect(stage.getByText("Authorization required", { exact: true })).toBeVisible();

  await center(page, "story-consent");
  await expect(stage).toHaveAttribute("data-step", "4");
  await expect(stage.getByText("Consent granted", { exact: true })).toBeVisible();

  await center(page, "story-retrieval");
  await expect(stage).toHaveAttribute("data-step", "5");
  await expect(stage.getByText("Secure view", { exact: true })).toBeVisible();
  await expect(stage.getByText("Owned by Hospital A", { exact: true })).toBeVisible();

  await center(page, "story-revocation");
  await expect(stage).toHaveAttribute("data-step", "8");
  await expect(stage.locator(".rc1-patient em")).toHaveText("Consent revoked");

  await center(page, "story-denied");
  await expect(stage).toHaveAttribute("data-step", "9");
  await expect(stage.getByText("Denied · consent inactive", { exact: true })).toBeVisible();
  await expect(stage.getByText("Audit & Integrity", { exact: true })).toBeVisible();
});

test("RC1 Continue and Replay are presentation-only controls", async ({ page }) => {
  await page.goto("/demo");
  await center(page, "story-authorization");
  const url = page.url();
  await page.getByRole("button", { name: /Replay/ }).click();
  await expect(page.locator(".rc1-stage")).toHaveAttribute("data-step", "3");
  expect(page.url()).toBe(url);
  await page.getByRole("button", { name: /Continue to next step/ }).click();
  await expect(page.getByTestId("story-consent")).toHaveAttribute("data-active", "true");
});

test("reduced motion retains complete RC1 meaning", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/demo");
  expect(await page.evaluate(() => matchMedia("(prefers-reduced-motion: reduce)").matches)).toBe(true);
  expect(await page.locator(".rc1-stage").evaluate((node) => getComputedStyle(node).position)).toBe("relative");
  for (const text of ["Hospital A", "Hospital B", "Patient P", "Clinical record", "Audit & Integrity"])
    await expect(page.getByText(text, { exact: false }).first()).toBeAttached();
});

test("RC2 uses safe backend labels and isolates the conceptual mismatch", async ({ page }) => {
  await page.goto("/authority/demo");
  const stage = page.locator(".rc2-stage");
  await expect(stage.getByText("Authority Event", { exact: true })).toBeVisible();
  for (const name of ["Hospital A", "Payer B", "Staffing Agency C"])
    await expect(stage.getByText(name, { exact: true })).not.toBeVisible();
  await expect(stage.locator(".rc2-local-response").first()).not.toBeVisible();
  await expect(stage.getByText("Recorded Evidence", { exact: true })).toHaveCount(0);
  await expect(stage.locator(".rc2-verification-summary")).toHaveCount(0);
  expect(await stage.locator(".rc2-story-connectors path").evaluateAll((nodes) => nodes.filter((node) => getComputedStyle(node).visibility === "visible").length)).toBe(0);

  await center(page, "authority-story-2");
  for (const name of ["Hospital A", "Payer B", "Staffing Agency C"])
    await expect(stage.getByText(name, { exact: true })).toBeVisible();
  await expect(stage.getByText("Recorded Evidence", { exact: true })).toHaveCount(0);
  expect(await stage.locator(".rc2-story-connectors path").evaluateAll((nodes) => nodes.filter((node) => getComputedStyle(node).visibility === "visible").length)).toBe(1);

  await center(page, "authority-story-3");
  for (const outcome of ["Scheduling disabled", "Enrollment/reimbursement held", "Assignment ended"])
    await expect(stage.getByText(outcome, { exact: true }).first()).toBeVisible();
  await expect(stage.locator(".rc2-local-response").first()).not.toBeVisible();
  await expect(page.getByText("Shared evidence does not mean shared decisions.", { exact: false })).toBeVisible();

  await center(page, "authority-story-4");
  await expect(stage.locator(".rc2-local-response")).toHaveCount(3);
  await expect(stage.locator(".rc2-local-response").first()).toBeVisible();
  await expect(stage.getByText("Recorded Evidence", { exact: true })).toHaveCount(0);
  expect(await stage.locator(".rc2-story-connectors path").evaluateAll((nodes) => nodes.filter((node) => getComputedStyle(node).visibility === "visible").length)).toBe(2);

  await center(page, "authority-story-5");
  await expect(stage.getByText("Recorded Evidence", { exact: true })).toBeVisible();
  await expect(stage.getByText("VERIFIED", { exact: true })).toHaveCount(0);
  expect(await stage.locator(".rc2-story-connectors path").evaluateAll((nodes) => nodes.filter((node) => getComputedStyle(node).visibility === "visible").length)).toBe(3);

  await center(page, "authority-story-6");
  await expect(stage).toHaveAttribute("data-step", "6");
  await expect(stage.getByText("VERIFIED", { exact: true })).toHaveCount(3);

  await center(page, "authority-story-8");
  await expect(stage).toHaveAttribute("data-step", "8");
  const columns = stage.locator(".rc2-organization-column");
  await expect(columns.nth(0)).toHaveClass(/is-mismatch/);
  await expect(columns.nth(1)).not.toHaveClass(/is-mismatch/);
  await expect(columns.nth(2)).not.toHaveClass(/is-mismatch/);
  await expect(columns.nth(0).getByText("FAILED", { exact: true })).toBeVisible();
  await expect(columns.nth(1).getByText("VERIFIED", { exact: true })).toBeVisible();
  await expect(columns.nth(2).getByText("VERIFIED", { exact: true })).toBeVisible();
  await expect(stage.getByText("Recorded Evidence", { exact: true })).toBeVisible();
  await expect(stage.getByText(/recorded evidence unchanged/i)).toBeVisible();
});

test("RC2 Continue and Replay never invoke mutation APIs", async ({ page }) => {
  const mutationRequests: string[] = [];
  page.on("request", (request) => { if (request.method() !== "GET") mutationRequests.push(request.url()); });
  await page.goto("/authority/demo");
  await center(page, "authority-story-3");
  const url = page.url();
  await page.getByRole("button", { name: /Replay/ }).click();
  await expect(page.locator(".rc2-stage")).toHaveAttribute("data-step", "3");
  expect(page.url()).toBe(url);
  await page.getByRole("button", { name: /Continue to next step/ }).click();
  await expect(page.getByTestId("authority-story-4")).toHaveAttribute("data-active", "true");
  expect(mutationRequests).toEqual([]);
});

test("reduced motion retains complete RC2 meaning", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/authority/demo");
  expect(await page.locator(".rc2-stage").evaluate((node) => getComputedStyle(node).position)).toBe("relative");
  for (const text of ["Authority Event", "Hospital A", "Payer B", "Staffing Agency C", "Recorded Evidence"])
    await expect(page.getByText(text, { exact: false }).first()).toBeAttached();
});

test("timeline rows keep stable geometry through forward and backward activation", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1050 });
  for (const { route, count, finalTestId } of [
    { route: "/demo", count: 9, finalTestId: "story-denied" },
    { route: "/authority/demo", count: 8, finalTestId: "authority-story-8" },
  ]) {
    await page.goto(route);
    const timeline = page.locator(".narrative-timeline");
    const steps = timeline.locator(".narrative-step");
    await expect(steps).toHaveCount(count);
    const initialGeometry = await steps.evaluateAll((nodes) => nodes.map((node) => ({ top: (node as HTMLElement).offsetTop, height: (node as HTMLElement).offsetHeight })));
    const markerCenters = await timeline.locator(".narrative-step-node").evaluateAll((nodes) => nodes.map((node) => { const box = node.getBoundingClientRect(); return box.left + box.width / 2; }));
    expect(Math.max(...markerCenters) - Math.min(...markerCenters)).toBeLessThanOrEqual(0.5);

    for (const ratio of [0.2, 0.4, 0.6, 0.8, 1]) {
      await timeline.evaluate((node, value) => node.scrollTo({ top: (node.scrollHeight - node.clientHeight) * value }), ratio);
      await page.waitForTimeout(100);
      await expect(timeline.locator(".narrative-step.is-active")).toHaveCount(1);
    }
    await expect(page.getByTestId(finalTestId)).toHaveAttribute("data-active", "true");
    for (const ratio of [0.8, 0.6, 0.4, 0.2, 0]) {
      await timeline.evaluate((node, value) => node.scrollTo({ top: (node.scrollHeight - node.clientHeight) * value }), ratio);
      await page.waitForTimeout(100);
      await expect(timeline.locator(".narrative-step.is-active")).toHaveCount(1);
    }
    await expect(steps.first()).toHaveAttribute("data-active", "true");

    await page.getByTestId(finalTestId).getByRole("button").click();
    await page.waitForTimeout(650);
    await expect(page.getByTestId(finalTestId)).toHaveAttribute("data-active", "true");
    await expect(timeline.locator(".narrative-step.is-active")).toHaveCount(1);
    expect(await steps.evaluateAll((nodes) => nodes.map((node) => ({ top: (node as HTMLElement).offsetTop, height: (node as HTMLElement).offsetHeight })))).toEqual(initialGeometry);

    await steps.first().getByRole("button").click();
    await page.waitForTimeout(650);
    await expect(steps.first()).toHaveAttribute("data-active", "true");
    await expect(timeline.locator(".narrative-step.is-active")).toHaveCount(1);
  }
});

test("final stage states keep contextual rows clear of evidence", async ({ page }) => {
  for (const viewport of [{ width: 1440, height: 1050 }, { width: 1024, height: 900 }, { width: 768, height: 1024 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport);
    await page.goto("/demo");
    await page.getByTestId("story-denied").getByRole("button").click();
    await expect(page.locator(".rc1-stage")).toHaveAttribute("data-step", "9");
    const [patient, rc1Evidence] = await Promise.all([page.locator(".rc1-patient").boundingBox(), page.locator(".rc1-stage .narrative-evidence-rail").boundingBox()]);
    expect(patient && rc1Evidence && patient.y + patient.height <= rc1Evidence.y).toBe(true);

    await page.goto("/authority/demo");
    await page.getByTestId("authority-story-8").getByRole("button").click();
    await expect(page.locator(".rc2-stage")).toHaveAttribute("data-step", "8");
    const [responses, status, rc2Evidence] = await Promise.all([page.locator(".rc2-organization-grid").boundingBox(), page.locator(".rc2-verification-summary").boundingBox(), page.locator(".rc2-stage .narrative-evidence-rail").boundingBox()]);
    expect(responses && status && responses.y + responses.height <= status.y).toBe(true);
    expect(status && rc2Evidence && status.y + status.height <= rc2Evidence.y).toBe(true);
  }
});

for (const route of ["/demo", "/authority/demo"])
  for (const viewport of [{ width: 1440, height: 1050 }, { width: 1024, height: 900 }, { width: 768, height: 1024 }, { width: 390, height: 844 }])
    test(`${route} has no overflow at ${viewport.width}`, async ({ page }) => {
      await page.setViewportSize(viewport);
      await page.goto(route);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
    });
