# RC2 safe response presentation projection

Phase 8.6 adds the minimum read-only presentation metadata needed to explain
the three independent organizational outcomes in the synthetic RC2 portfolio
demo. It does not expose the persisted policy, decision, or action models.

## Exposed shape

Completed responses belonging to the exact synthetic Phase 8 fixture may add:

```json
{
  "actionPresentation": {
    "code": "SCHEDULING_DISABLED",
    "label": "Scheduling disabled",
    "demoOnly": true
  }
}
```

The bounded server mapping is keyed by the persisted organization and action
identities. The three allowlisted results are:

| Organization | Persisted action ID | Safe code | Safe label |
| --- | --- | --- | --- |
| Hospital A | `ACT-P8-HOSP` | `SCHEDULING_DISABLED` | Scheduling disabled |
| Payer B | `ACT-P8-PAYER` | `ENROLLMENT_HELD` | Enrollment/reimbursement held |
| Staffing Agency C | `ACT-P8-STAFF` | `ASSIGNMENT_ENDED` | Assignment ended |

An unknown identity, a mismatched organization/action pair, or a response that
has not reached `ACTION_COMPLETED` receives no presentation projection. These
labels are explicitly demo-only metadata; they are not a general healthcare
action taxonomy or a claim about a real organization.

## Privacy and integrity boundary

The projection does not serialize policy text, decision or action explanation,
reviewer or operator references, supporting evidence, source documents, or raw
persisted models. It adds no endpoint and no write capability.

No fixture value, database schema, canonical encoder, commitment, contract,
submission, verification, or response transition changes. The mapping is
applied only while constructing the existing HTTP read view, after persisted
response identity and state have been loaded. Consequently it has no effect on
the evidence committed to the ResponseLedger.

## Limitations and non-claims

The projection is bounded synthetic presentation data for the RC2 portfolio
demo. It does not establish practitioner-validated terminology, production
policy semantics, real organizational action correctness, production API
completeness, production authorization, or regulatory compliance.
