import{expect,test}from"@playwright/test";

const authorityUrl="http://localhost:3000/authority";

test("real RC2 authority-response integrity hero",async({page})=>{
  const mutationRequests:string[]=[];
  page.on("request",request=>{if(request.url().endsWith("/api/rc2/demo/tamper"))mutationRequests.push(request.postData()??"")});
  await page.goto(authorityUrl);
  await expect(page.getByRole("heading",{name:"One event. Three independent responses."})).toBeVisible();
  await expect(page.getByText("Demonstration data")).toBeVisible();
  await expect(page.getByText("Scheduling disabled")).toBeVisible();
  await expect(page.getByText("Enrollment/reimbursement held")).toBeVisible();
  await expect(page.getByText("Assignment ended")).toBeVisible();
  for(const name of["Hospital A","Payer B","Staffing Agency C"]){
    await page.getByRole("button",{name:new RegExp(name)}).click();
    await expect(page.getByRole("list",{name:"Response lifecycle"})).toBeVisible();
  }
  await page.getByRole("button",{name:"Technical",exact:true}).click();
  await expect(page.getByRole("heading",{name:"Evidence anchored across a permissioned network."})).toBeVisible();
  await expect(page.getByText("Static deployment configuration")).toBeVisible();
  await expect(page.getByRole("region",{name:"Ledger activity"})).toContainText("Confirmed");
  await page.getByRole("link",{name:"Integrity demo"}).click();
  await expect(page.getByRole("heading",{name:"Show that changed evidence is detected"})).toBeVisible();
  await page.getByRole("button",{name:"Simulate local policy change"}).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(mutationRequests).toHaveLength(0);
  await expect(page.getByRole("button",{name:"Cancel"})).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button",{name:"Simulate local policy change"}).click();
  await page.getByRole("button",{name:"Confirm simulation"}).click();
  await expect.poll(()=>mutationRequests.length).toBe(1);
  expect(mutationRequests[0]).toBe('{"target":"HOSPITAL_A_POLICY"}');
  await expect(page.getByRole("heading",{name:"A difference has been detected"})).toBeVisible();
  const statuses=page.locator(".integrity-statuses");
  await expect(statuses).toContainText("Hospital A");
  await expect(statuses).toContainText("Evidence no longer matches");
  await expect(statuses).toContainText("Payer B");
  await expect(statuses).toContainText("Staffing Agency C");
  await expect(statuses.getByText("Evidence verified")).toHaveCount(2);
  await expect(page.getByText("Recorded ledger evidence remained unchanged.")).toBeVisible();
  await page.reload();
  await page.getByRole("button",{name:"Technical",exact:true}).click();
  await expect(page.getByRole("heading",{name:"A difference has been detected"})).toBeVisible();
  await expect(page.getByRole("button",{name:"Simulate local policy change"})).toBeDisabled();
});
