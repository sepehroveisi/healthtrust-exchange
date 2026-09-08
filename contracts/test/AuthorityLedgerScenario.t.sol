// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { TestBase } from "./TestBase.sol";
import { OrganizationRegistry } from "../src/OrganizationRegistry.sol";
import { AuthorityRegistry } from "../src/AuthorityRegistry.sol";
import { AuthorityEventRegistry } from "../src/AuthorityEventRegistry.sol";
import { ResponseLedger } from "../src/ResponseLedger.sol";

contract AuthorityLedgerScenarioTest is TestBase {
    bytes32 private constant HOSPITAL = bytes32("HOSPITAL-A");
    bytes32 private constant PAYER = bytes32("PAYER-B");
    bytes32 private constant STAFFING = bytes32("STAFFING-AGENCY-C");
    bytes32 private constant AUTHORITY = bytes32("HHS-OIG-DEMO");
    bytes32 private constant SERIES = bytes32("EXCLUSION-SERIES-1");
    bytes32 private constant ORIGINAL = bytes32("EXCLUSION-ORIGINAL-1");
    bytes32 private constant REINSTATEMENT = bytes32("REINSTATEMENT-1");
    address private constant AUTHORITY_SUBMITTER = address(0xA17);
    address private constant HOSPITAL_SUBMITTER = address(0xA);
    address private constant PAYER_SUBMITTER = address(0xB);
    address private constant STAFFING_SUBMITTER = address(0xC);
    int64 private constant RECEIPT_TIME = 1_767_225_600_000_000_000;

    OrganizationRegistry private organizations;
    AuthorityRegistry private authorities;
    AuthorityEventRegistry private events;
    ResponseLedger private responses;

    function setUp() public {
        organizations = new OrganizationRegistry(address(this));
        authorities = new AuthorityRegistry(address(this));
        events = new AuthorityEventRegistry(authorities);
        responses = new ResponseLedger(organizations, events);

        organizations.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, HOSPITAL_SUBMITTER
        );
        organizations.registerOrganization(
            PAYER, OrganizationRegistry.OrganizationType.PAYER, PAYER_SUBMITTER
        );
        organizations.registerOrganization(
            STAFFING, OrganizationRegistry.OrganizationType.STAFFING_AGENCY, STAFFING_SUBMITTER
        );
        authorities.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, AUTHORITY_SUBMITTER
        );
    }

    function testThreeIndependentResponsesSurviveReinstatement() public {
        bytes32 originalCommitment = sha256("synthetic exclusion assertion");
        vm.prank(AUTHORITY_SUBMITTER);
        events.recordAssertion(
            AuthorityEventRegistry.AuthorityAssertion({
                eventId: ORIGINAL,
                eventSeriesId: SERIES,
                previousEventId: bytes32(0),
                targetEventId: bytes32(0),
                authorityId: AUTHORITY,
                eventCommitment: originalCommitment,
                eventType: AuthorityEventRegistry.EventType.EXCLUSION,
                assertionKind: AuthorityEventRegistry.AssertionKind.ORIGINAL,
                authorityEffect: AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE,
                effectiveTime: RECEIPT_TIME - 1,
                recordedAt: 0
            })
        );

        bytes32 hospitalHead = completeResponse(HOSPITAL, HOSPITAL_SUBMITTER, "hospital");
        bytes32 payerHead = completeResponse(PAYER, PAYER_SUBMITTER, "payer");
        bytes32 staffingHead = completeResponse(STAFFING, STAFFING_SUBMITTER, "staffing");

        assertTrue(
            responses.getResponseVersion(hospitalHead).decisionCommitment
                != responses.getResponseVersion(payerHead).decisionCommitment
        );
        assertTrue(
            responses.getResponseVersion(payerHead).actionCommitment
                != responses.getResponseVersion(staffingHead).actionCommitment
        );
        assertEq(responses.historyLength(ORIGINAL, HOSPITAL), 4);
        assertEq(responses.historyLength(ORIGINAL, PAYER), 4);
        assertEq(responses.historyLength(ORIGINAL, STAFFING), 4);

        bytes32 reinstatementCommitment = sha256("synthetic reinstatement assertion");
        vm.prank(AUTHORITY_SUBMITTER);
        events.recordAssertion(
            AuthorityEventRegistry.AuthorityAssertion({
                eventId: REINSTATEMENT,
                eventSeriesId: SERIES,
                previousEventId: ORIGINAL,
                targetEventId: ORIGINAL,
                authorityId: AUTHORITY,
                eventCommitment: reinstatementCommitment,
                eventType: AuthorityEventRegistry.EventType.EXCLUSION,
                assertionKind: AuthorityEventRegistry.AssertionKind.REINSTATEMENT,
                authorityEffect: AuthorityEventRegistry.AuthorityEffect.EXCLUSION_LIFTED,
                effectiveTime: RECEIPT_TIME + 1,
                recordedAt: 0
            })
        );

        assertEq(events.historyLength(SERIES), 2);
        assertEq(events.historyEventId(SERIES, 0), ORIGINAL);
        assertEq(events.currentHead(SERIES), REINSTATEMENT);
        assertEq(events.getAssertion(ORIGINAL).eventCommitment, originalCommitment);
        assertEq(responses.latestResponseVersionId(ORIGINAL, HOSPITAL), hospitalHead);
        assertEq(responses.latestResponseVersionId(ORIGINAL, PAYER), payerHead);
        assertEq(responses.latestResponseVersionId(ORIGINAL, STAFFING), staffingHead);
        assertEq(responses.historyLength(ORIGINAL, HOSPITAL), 4);
        assertEq(responses.historyLength(ORIGINAL, PAYER), 4);
        assertEq(responses.historyLength(ORIGINAL, STAFFING), 4);
    }

    function completeResponse(bytes32 organizationId, address submitter, bytes memory seed)
        private
        returns (bytes32)
    {
        bytes32 responseId = keccak256(abi.encode("response", organizationId));
        bytes32 v1 = keccak256(abi.encode(responseId, uint8(1)));
        bytes32 v2 = keccak256(abi.encode(responseId, uint8(2)));
        bytes32 v3 = keccak256(abi.encode(responseId, uint8(3)));
        bytes32 v4 = keccak256(abi.encode(responseId, uint8(4)));
        bytes32 policy = sha256(abi.encode(seed, "policy"));
        bytes32 decision = sha256(abi.encode(seed, "decision"));
        bytes32 action = sha256(abi.encode(seed, "action"));

        record(submitter, response(responseId, v1, 0, organizationId, 1, 0, 0, 0));
        record(submitter, response(responseId, v2, v1, organizationId, 2, policy, 0, 0));
        record(submitter, response(responseId, v3, v2, organizationId, 3, policy, decision, 0));
        record(submitter, response(responseId, v4, v3, organizationId, 4, policy, decision, action));
        return v4;
    }

    function response(
        bytes32 responseId,
        bytes32 versionId,
        bytes32 previous,
        bytes32 organizationId,
        uint8 state,
        bytes32 policy,
        bytes32 decision,
        bytes32 action
    ) private pure returns (ResponseLedger.ResponseVersion memory) {
        return ResponseLedger.ResponseVersion({
            responseId: responseId,
            responseVersionId: versionId,
            previousResponseVersionId: previous,
            eventId: ORIGINAL,
            organizationId: organizationId,
            responseState: ResponseLedger.ResponseState(state),
            receiptTimestamp: RECEIPT_TIME,
            policyVersionHash: policy,
            decisionCommitment: decision,
            actionCommitment: action,
            recordedAt: 0
        });
    }

    function record(address submitter, ResponseLedger.ResponseVersion memory value) private {
        vm.prank(submitter);
        responses.recordResponseVersion(value);
    }
}
