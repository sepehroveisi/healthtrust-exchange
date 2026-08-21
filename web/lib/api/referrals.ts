import { api } from "./client";

export type Referral = {
  ID: string;
  PatientID: string;
  NetworkPatientID: string;
  RecordID: string;
  SourceOrganizationID: string;
  DestinationOrganizationID: string;
  CreatedByActorID: string;
  Status: string;
  CreatedAt: string;
  CommitState: string;
};

type Incoming = { Referral: Referral; TransactionID: string; ReceivedAt: string };

export const referralsApi = {
  list: (patientId = "") => api<Referral[]>("a", `/referrals?patientId=${encodeURIComponent(patientId)}`),
  incoming: async (patientId = "") => (await api<Incoming[]>("b", `/incoming-referrals?patientId=${encodeURIComponent(patientId)}`)).map(value => value.Referral),
  create: (value: Referral, key: string) => api<Referral>("a", "/referrals", { method: "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify(value) }),
};
