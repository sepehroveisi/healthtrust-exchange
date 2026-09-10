export const hospitals={a:process.env.NEXT_PUBLIC_HOSPITAL_A_API_URL??"http://localhost:8081",b:process.env.NEXT_PUBLIC_HOSPITAL_B_API_URL??"http://localhost:8082"} as const;
export const rc2ApiUrl=process.env.NEXT_PUBLIC_RC2_API_URL??"http://localhost:8090";
export type HospitalKey=keyof typeof hospitals;
export const demoRoles={doctorA:{actorId:"doctor-a",organizationId:"hospital-a",hospital:"a"},doctorB:{actorId:"doctor-b",organizationId:"hospital-b",hospital:"b"},patient:{actorId:"patient-p",organizationId:"hospital-a",hospital:"a"},staffA:{actorId:"staff-a",organizationId:"hospital-a",hospital:"a"},staffB:{actorId:"staff-b",organizationId:"hospital-b",hospital:"b"}} as const;
