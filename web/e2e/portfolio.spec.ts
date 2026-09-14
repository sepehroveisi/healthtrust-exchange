import{expect,test}from"@playwright/test";

test("unified landing preserves both prototype paths",async({page})=>{
  await page.goto("/");
  await expect(page.getByRole("heading",{name:/Two prototypes/})).toBeVisible();
  await expect(page.locator("#evolution")).toContainText("Question the architecture");
  await expect(page.locator("#rc1")).toContainText("Patient-controlled access");
  await expect(page.locator("#rc2")).toContainText("Shared evidence. Independent decisions.");
  await page.getByRole("link",{name:/Explore RC1/}).click();
  await expect(page).toHaveURL(/\/app$/);
  await page.goto("/");
  await page.getByRole("link",{name:/Explore RC2/}).click();
  await expect(page).toHaveURL(/\/authority$/);
});

for(const viewport of[{width:1440,height:1050},{width:1024,height:900},{width:768,height:1024},{width:390,height:844}])test(`portfolio has no horizontal overflow at ${viewport.width}px`,async({page})=>{
  await page.setViewportSize(viewport);await page.goto("/");
  await page.locator("#comparison").scrollIntoViewIfNeeded();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  for(const image of await page.locator(".portfolio-screen img").all()){
    await image.scrollIntoViewIfNeeded();
    await expect.poll(()=>image.evaluate(node=>(node as HTMLImageElement).complete&&(node as HTMLImageElement).naturalWidth>0)).toBe(true);
  }
});
