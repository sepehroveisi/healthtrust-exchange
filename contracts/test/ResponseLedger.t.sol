// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { TestBase } from "./TestBase.sol";
import { OrganizationRegistry } from "../src/OrganizationRegistry.sol";
import { AuthorityRegistry } from "../src/AuthorityRegistry.sol";
import { AuthorityEventRegistry } from "../src/AuthorityEventRegistry.sol";
import { ResponseLedger } from "../src/ResponseLedger.sol";

contract ResponseLedgerTest is TestBase {
    OrganizationRegistry private organizations;
    AuthorityRegistry private authorities;
    AuthorityEventRegistry private events;
    ResponseLedger private responses;

    bytes32 private constant AUTHORITY = bytes32("HHS-OIG-DEMO");
    bytes32 private constant EVENT = bytes32("EVENT-1");
    bytes32 private constant SERIES = bytes32("SERIES-1");
    bytes32 private constant HOSPITAL = bytes32("HOSPITAL-A");
    bytes32 private constant RESPONSE = bytes32("RESPONSE-HOSPITAL");
    bytes32 private constant V1 = bytes32("RESPONSE-H-V1");
    bytes32 private constant V2 = bytes32("RESPONSE-H-V2");
    bytes32 private constant V3 = bytes32("RESPONSE-H-V3");
    bytes32 private constant V4 = bytes32("RESPONSE-H-V4");
    bytes32 private constant POLICY = sha256("policy");
    bytes32 private constant DECISION = sha256("decision");
    bytes32 private constant ACTION = sha256("action");
    address private constant AUTHORITY_SUBMITTER = address(0xA17);
    address private constant HOSPITAL_SUBMITTER = address(0xA11CE);
    int64 private constant RECEIVED_AT = 1_767_225_600_000_000_000;

    function setUp() public {
        organizations = new OrganizationRegistry(address(this));
        authorities = new AuthorityRegistry(address(this));
        events = new AuthorityEventRegistry(authorities);
        responses = new ResponseLedger(organizations, events);
        organizations.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, HOSPITAL_SUBMITTER
        );
        authorities.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, AUTHORITY_SUBMITTER
        );
        bytes32 eventCommitment = sha256("event");
        vm.prank(AUTHORITY_SUBMITTER);
        events.recordAssertion(
            AuthorityEventRegistry.AuthorityAssertion({
                eventId: EVENT,
                eventSeriesId: SERIES,
                previousEventId: bytes32(0),
                targetEventId: bytes32(0),
                authorityId: AUTHORITY,
                eventCommitment: eventCommitment,
                eventType: AuthorityEventRegistry.EventType.EXCLUSION,
                assertionKind: AuthorityEventRegistry.AssertionKind.ORIGINAL,
                authorityEffect: AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE,
                effectiveTime: RECEIVED_AT - 1,
                recordedAt: 0
            })
        );
    }

    function version(
        bytes32 versionId,
        bytes32 previous,
        ResponseLedger.ResponseState state,
        bytes32 policy,
        bytes32 decision,
        bytes32 action
    ) private pure returns (ResponseLedger.ResponseVersion memory) {
        return ResponseLedger.ResponseVersion({
            responseId: RESPONSE,
            responseVersionId: versionId,
            previousResponseVersionId: previous,
            eventId: EVENT,
            organizationId: HOSPITAL,
            responseState: state,
            receiptTimestamp: RECEIVED_AT,
            policyVersionHash: policy,
            decisionCommitment: decision,
            actionCommitment: action,
            recordedAt: 0
        });
    }

    function submit(ResponseLedger.ResponseVersion memory value) private {
        vm.prank(HOSPITAL_SUBMITTER);
        responses.recordResponseVersion(value);
    }

    function received() private {
        submit(version(V1, bytes32(0), ResponseLedger.ResponseState.RECEIVED, 0, 0, 0));
    }

    function underReview() private {
        received();
        submit(version(V2, V1, ResponseLedger.ResponseState.UNDER_REVIEW, POLICY, 0, 0));
    }

    function decided() private {
        underReview();
        submit(version(V3, V2, ResponseLedger.ResponseState.DECIDED, POLICY, DECISION, 0));
    }

    function testCreatesReceivedVersion() public {
        received();
        ResponseLedger.ResponseVersion memory stored = responses.getResponseVersion(V1);
        assertEq(stored.responseId, RESPONSE);
        assertEq(uint256(stored.responseState), 1);
        assertEq(stored.receiptTimestamp, RECEIVED_AT);
        assertEq(responses.latestResponseVersionId(EVENT, HOSPITAL), V1);
    }

    function testCompletesStrictLifecycleAndPreservesHistory() public {
        decided();
        submit(
            version(V4, V3, ResponseLedger.ResponseState.ACTION_COMPLETED, POLICY, DECISION, ACTION)
        );
        assertEq(responses.historyLength(EVENT, HOSPITAL), 4);
        assertEq(responses.historyResponseVersionId(EVENT, HOSPITAL, 0), V1);
        assertEq(responses.historyResponseVersionId(EVENT, HOSPITAL, 3), V4);
        assertEq(responses.latestResponseVersionId(EVENT, HOSPITAL), V4);
        assertEq(uint256(responses.getResponseVersion(V1).responseState), 1);
        assertEq(uint256(responses.getResponseVersion(V4).responseState), 4);
    }

    function testRejectsSkippedTransition() public {
        received();
        ResponseLedger.ResponseVersion memory value =
            version(V3, V1, ResponseLedger.ResponseState.DECIDED, POLICY, DECISION, 0);
        vm.prank(HOSPITAL_SUBMITTER);
        vm.expectPartialRevert(ResponseLedger.InvalidTransition.selector);
        responses.recordResponseVersion(value);
    }

    function testRejectsBackwardTransition() public {
        underReview();
        ResponseLedger.ResponseVersion memory value =
            version(V3, V2, ResponseLedger.ResponseState.RECEIVED, 0, 0, 0);
        vm.prank(HOSPITAL_SUBMITTER);
        vm.expectPartialRevert(ResponseLedger.ResponseStreamAlreadyExists.selector);
        responses.recordResponseVersion(value);
    }

    function testRejectsWrongPreviousVersion() public {
        received();
        ResponseLedger.ResponseVersion memory value = version(
            V2, bytes32("WRONG"), ResponseLedger.ResponseState.UNDER_REVIEW, POLICY, 0, 0
        );
        vm.prank(HOSPITAL_SUBMITTER);
        vm.expectPartialRevert(ResponseLedger.WrongPreviousVersion.selector);
        responses.recordResponseVersion(value);
    }

    function testRejectsDuplicateVersionIdentity() public {
        received();
        ResponseLedger.ResponseVersion memory value =
            version(V1, V1, ResponseLedger.ResponseState.UNDER_REVIEW, POLICY, 0, 0);
        vm.prank(HOSPITAL_SUBMITTER);
        vm.expectPartialRevert(ResponseLedger.ResponseVersionAlreadyRegistered.selector);
        responses.recordResponseVersion(value);
    }

    function testRejectsUnauthorizedOrganization() public {
        ResponseLedger.ResponseVersion memory value =
            version(V1, bytes32(0), ResponseLedger.ResponseState.RECEIVED, 0, 0, 0);
        vm.prank(address(0xBAD));
        vm.expectPartialRevert(ResponseLedger.UnauthorizedOrganizationSubmitter.selector);
        responses.recordResponseVersion(value);
    }

    function testRejectsChangedAccumulatedCommitment() public {
        underReview();
        ResponseLedger.ResponseVersion memory value =
            version(V3, V2, ResponseLedger.ResponseState.DECIDED, sha256("changed"), DECISION, 0);
        vm.prank(HOSPITAL_SUBMITTER);
        vm.expectPartialRevert(ResponseLedger.InvalidSnapshot.selector);
        responses.recordResponseVersion(value);
    }

    function testRejectsVersionAfterActionCompleted() public {
        decided();
        submit(
            version(V4, V3, ResponseLedger.ResponseState.ACTION_COMPLETED, POLICY, DECISION, ACTION)
        );
        ResponseLedger.ResponseVersion memory value = version(
            bytes32("V5"),
            V4,
            ResponseLedger.ResponseState.ACTION_COMPLETED,
            POLICY,
            DECISION,
            ACTION
        );
        vm.prank(HOSPITAL_SUBMITTER);
        vm.expectPartialRevert(ResponseLedger.InvalidTransition.selector);
        responses.recordResponseVersion(value);
    }
}
