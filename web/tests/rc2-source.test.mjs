import assert from"node:assert/strict";
import{readFile}from"node:fs/promises";
import test from"node:test";

const componentUrl=new URL("../components/rc2/authority-workspace.tsx",import.meta.url);
const clientUrl=new URL("../lib/rc2/client.ts",import.meta.url);

test("RC2 workspace preserves honest presentation and trust boundaries",async()=>{
  const[component,client]=await Promise.all([readFile(componentUrl,"utf8"),readFile(clientUrl,"utf8")]);
  for(const phrase of["Operations","Technical","Demonstration data","One event. Three independent responses.","Static deployment configuration","not live peer health","Current canonical policy evidence","Previously recorded policy evidence","Verification unavailable","Evidence no longer matches","Recorded ledger evidence remained unchanged","HOSPITAL_A_POLICY"])assert.ok(component.includes(phrase),`missing ${phrase}`);
  assert.match(component,/response\.actionPresentation\?\.label/);
  assert.match(component,/Outcome not available/);
  assert.match(component,/loadRC2Workspace/);
  assert.match(component,/await simulateHospitalPolicyChange\(\)[^]*await reload\(\)/);
  assert.match(component,/role="dialog"/);
  assert.match(component,/event\.key==="Escape"/);
  assert.doesNotMatch(component,/peer count|transactions per second|TPS|validator health/i);
  assert.match(client,/JSON\.stringify\(\{target:"HOSPITAL_A_POLICY"\}\)/);
  assert.doesNotMatch(client,/function simulateHospitalPolicyChange\([^)]{1,}/);
  assert.doesNotMatch(client,/8545|8546|8547|besu/i);
});
