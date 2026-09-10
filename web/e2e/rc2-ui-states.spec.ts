import{expect,test,type Page}from"@playwright/test";

const authorityUrl="http://localhost:3000/authority";
const orgs=["HOSPITAL-A","PAYER-B","STAFFING-AGENCY-C"];
const names:Record<string,string>={"HOSPITAL-A":"Hospital A","PAYER-B":"Payer B","STAFFING-AGENCY-C":"Staffing Agency C"};
const now="2026-07-01T00:00:00.000Z";
const ledger={state:"CONFIRMED",operationId:"OP-DEMO",transactionHash:"//////////////////////////////////////////////////////////////////////////8=",blockNumber:123456,blockHash:"/////////////////////////////////////////////////////////////////////////////////////w=="};
const event={ID:"EVENT-FROM-API",SeriesID:"SERIES-PHASE8",AuthorityID:"HHS-OIG-DEMO",SubjectID:"PRV-7F31A",EventType:"EXCLUSION",AssertionKind:"ORIGINAL",Effect:"EXCLUSION_ACTIVE",EffectiveTime:now,CurrentHead:"EVENT-FROM-API",Commitment:Array(32).fill(255),Ledger:ledger,CreatedAt:now};
const states=["RECEIVED","UNDER_REVIEW","DECIDED","ACTION_COMPLETED"];
const outcomes=["Scheduling disabled","Enrollment/reimbursement held","Assignment ended"];

function fixture(status:"VERIFIED"|"FAILED"|"INDETERMINATE"="VERIFIED",missingOutcome=false){
  const responses=orgs.map((organization,index)=>({ID:`RESP-${index}-V4`,ResponseID:`RESP-${index}`,EventID:event.ID,OrganizationID:organization,State:"ACTION_COMPLETED",PreviousID:`RESP-${index}-V3`,PolicyVersionID:`POL-${index}-V1`,DecisionID:`DEC-${index}`,ActionID:`ACT-${index}`,Commitment:Array(32).fill(index+1),Ledger:{...ledger,blockNumber:200+index},CreatedAt:now,...(missingOutcome&&index===2?{}:{actionPresentation:{code:`ACTION_${index}`,label:outcomes[index],demoOnly:true}})}));
  const histories=Object.fromEntries(orgs.map((organization,index)=>[organization,states.map((state,version)=>({...responses[index],ID:`RESP-${index}-V${version+1}`,State:state,PreviousID:version?`RESP-${index}-V${version}`:undefined,CreatedAt:new Date(Date.parse(now)+version*60000).toISOString()}))]));
  const result=(organization:string)=>({status,objectType:"ORGANIZATION_RESPONSE",objectId:organization,checks:[{type:"CANONICAL_COMMITMENT",status,objectId:`${organization}:policy`,expected:"0x"+"a".repeat(64),observed:"0x"+(status==="FAILED"?"b":"a").repeat(64)}]});
  const bundle={status,eventId:event.ID,authority:{status,objectType:"AUTHORITY_EVENT",objectId:event.ID,checks:[]},responses:orgs.map(organization=>({organizationId:organization,result:result(organization)}))};
  return{responses,histories,bundle,verifications:Object.fromEntries(orgs.map(organization=>[organization,result(organization)]))};
}

async function mockAPI(page:Page,options:{status?:"VERIFIED"|"FAILED"|"INDETERMINATE";missingOutcome?:boolean;tamperCode?:number;delay?:number;eventCode?:number;emptyResponses?:boolean}={}){
  const data=fixture(options.status,options.missingOutcome);
  await page.route("**/api/rc2/**",async route=>{
    if(options.delay)await new Promise(resolve=>setTimeout(resolve,options.delay));
    const url=new URL(route.request().url()),path=url.pathname;
    const headers={"content-type":"application/json","access-control-allow-origin":"http://localhost:3000"};
    if(route.request().method()==="POST")return route.fulfill({status:options.tamperCode??403,headers,body:JSON.stringify({error:"demo_disabled"})});
    if(options.eventCode&&path.endsWith("/EVENT-PHASE8"))return route.fulfill({status:options.eventCode,headers,body:JSON.stringify({error:"runtime_unavailable"})});
    let body:unknown=event;
    if(path.endsWith("/verification/bundle"))body=data.bundle;
    else if(path.endsWith("/responses"))body=options.emptyResponses?[]:data.responses;
    else if(path.endsWith("/history")){const organization=orgs.find(org=>path.includes(`/responses/${org}/`))!;body=data.histories[organization]}
    else if(path.endsWith("/verification")){const organization=orgs.find(org=>path.includes(`/responses/${org}/`))!;body=data.verifications[organization]}
    const responseStatus=options.status==="INDETERMINATE"&&path.includes("verification")?503:200;
    return route.fulfill({status:responseStatus,headers,body:JSON.stringify(body)});
  });
}

test("operations and technical views use API values and honest fallback",async({page})=>{
  await mockAPI(page,{missingOutcome:true});await page.goto(authorityUrl);
  await expect(page.getByText("EVENT-FROM-API",{exact:true}).first()).toBeVisible();
  for(const name of Object.values(names))await expect(page.getByText(name).first()).toBeVisible();
  await expect(page.getByText("Scheduling disabled")).toBeVisible();
  await expect(page.getByText("Enrollment/reimbursement held")).toBeVisible();
  await expect(page.locator(".response-card").nth(2).locator(".response-outcome strong")).toHaveText("Action completed");
  await page.getByRole("button",{name:"Technical",exact:true}).click();
  await expect(page.getByText("Static deployment configuration")).toBeVisible();
  await expect(page.getByText("not live peer health")).toBeVisible();
  await expect(page.getByRole("region",{name:"Ledger activity"})).toContainText("123456");
});

for(const status of["FAILED","INDETERMINATE"] as const)test(`${status} is rendered as its own domain state`,async({page})=>{
  await mockAPI(page,{status});await page.goto(authorityUrl);
  await expect(page.getByText(status==="FAILED"?"Evidence no longer matches":"Verification unavailable").first()).toBeVisible();
});

test("loading and differentiated runtime error states are visible",async({page})=>{
  await mockAPI(page,{delay:500});const navigation=page.goto(authorityUrl);await expect(page.getByText("Loading authoritative event evidence…")).toBeVisible();await navigation;
  await page.unrouteAll({behavior:"wait"});await mockAPI(page,{eventCode:500});await page.reload();
  await expect(page.getByRole("heading",{name:"Authority service unavailable"})).toBeVisible();
});

test("disabled integrity demo is confirmation gated and keyboard operable",async({page})=>{
  await mockAPI(page,{tamperCode:403});await page.goto(authorityUrl);await expect(page.getByRole("heading",{name:"One event. Three independent responses."})).toBeVisible();await page.getByRole("button",{name:"Technical",exact:true}).click();await expect(page.getByRole("heading",{name:"Evidence anchored across a permissioned network."})).toBeVisible();
  await page.getByRole("button",{name:"Simulate local policy change"}).focus();await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button",{name:"Confirm simulation"}).click();
  await expect(page.getByText("The backend has not enabled the controlled integrity demonstration.")).toBeVisible();
  await page.keyboard.press("Escape");await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("empty response state is intentional",async({page})=>{await mockAPI(page,{emptyResponses:true});await page.goto(authorityUrl);await expect(page.getByRole("status")).toContainText("No organization responses yet")});

for(const viewport of[{width:1440,height:1000},{width:1180,height:820},{width:760,height:1000}])test(`layout has no page overflow at ${viewport.width}px`,async({page})=>{
  await page.setViewportSize(viewport);await mockAPI(page);await page.goto(authorityUrl);
  await page.getByRole("button",{name:"Technical",exact:true}).click();
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(1);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
