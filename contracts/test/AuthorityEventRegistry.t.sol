// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { TestBase } from "./TestBase.sol";
import { AuthorityRegistry } from "../src/AuthorityRegistry.sol";
import { AuthorityEventRegistry } from "../src/AuthorityEventRegistry.sol";

contract AuthorityEventRegistryTest is TestBase {
    AuthorityRegistry private authorities;
    AuthorityEventRegistry private events;

    bytes32 private constant AUTHORITY = bytes32("HHS-OIG-DEMO");
    bytes32 private constant SERIES = bytes32("EXCLUSION-SERIES-1");
    bytes32 private constant OTHER_SERIES = bytes32("EXCLUSION-SERIES-2");
    bytes32 private constant E1 = bytes32("EVENT-1");
    bytes32 private constant E2 = bytes32("EVENT-2");
    bytes32 private constant E3 = bytes32("EVENT-3");
    bytes32 private constant E4 = bytes32("EVENT-4");
    address private constant AUTHORITY_SUBMITTER = address(0xA17);

    function setUp() public {
        authorities = new AuthorityRegistry(address(this));
        authorities.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, AUTHORITY_SUBMITTER
        );
        events = new AuthorityEventRegistry(authorities);
    }

    function assertion(
        bytes32 eventId,
        bytes32 seriesId,
        bytes32 previous,
        bytes32 target,
        AuthorityEventRegistry.AssertionKind kind,
        AuthorityEventRegistry.AuthorityEffect effect
    ) private pure returns (AuthorityEventRegistry.AuthorityAssertion memory) {
        return AuthorityEventRegistry.AuthorityAssertion({
            eventId: eventId,
            eventSeriesId: seriesId,
            previousEventId: previous,
            targetEventId: target,
            authorityId: AUTHORITY,
            eventCommitment: sha256(abi.encode(eventId)),
            eventType: AuthorityEventRegistry.EventType.EXCLUSION,
            assertionKind: kind,
            authorityEffect: effect,
            effectiveTime: 1_767_225_600_000_000_000,
            recordedAt: 0
        });
    }

    function submit(AuthorityEventRegistry.AuthorityAssertion memory value) private {
        vm.prank(AUTHORITY_SUBMITTER);
        events.recordAssertion(value);
    }

    function original() private {
        submit(
            assertion(
                E1,
                SERIES,
                bytes32(0),
                bytes32(0),
                AuthorityEventRegistry.AssertionKind.ORIGINAL,
                AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
            )
        );
    }

    function testRecordsOriginalExclusionAssertion() public {
        original();
        AuthorityEventRegistry.AuthorityAssertion memory stored = events.getAssertion(E1);
        assertEq(stored.eventId, E1);
        assertEq(stored.eventSeriesId, SERIES);
        assertEq(uint256(stored.assertionKind), 1);
        assertEq(uint256(stored.authorityEffect), 1);
        assertEq(events.currentHead(SERIES), E1);
        assertEq(events.historyLength(SERIES), 1);
    }

    function testAppendsValidCorrectionSupersessionAndReinstatement() public {
        original();
        submit(
            assertion(
                E2,
                SERIES,
                E1,
                E1,
                AuthorityEventRegistry.AssertionKind.CORRECTION,
                AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
            )
        );
        submit(
            assertion(
                E3,
                SERIES,
                E2,
                E2,
                AuthorityEventRegistry.AssertionKind.SUPERSESSION,
                AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
            )
        );
        submit(
            assertion(
                E4,
                SERIES,
                E3,
                E3,
                AuthorityEventRegistry.AssertionKind.REINSTATEMENT,
                AuthorityEventRegistry.AuthorityEffect.EXCLUSION_LIFTED
            )
        );
        assertEq(events.currentHead(SERIES), E4);
        assertEq(events.historyLength(SERIES), 4);
        assertEq(events.historyEventId(SERIES, 0), E1);
        assertEq(events.historyEventId(SERIES, 3), E4);
        assertEq(uint256(events.getAssertion(E1).authorityEffect), 1);
        assertEq(uint256(events.getAssertion(E4).authorityEffect), 2);
    }

    function testRejectsDuplicateAssertionIdentity() public {
        AuthorityEventRegistry.AuthorityAssertion memory value = assertion(
            E1,
            SERIES,
            bytes32(0),
            bytes32(0),
            AuthorityEventRegistry.AssertionKind.ORIGINAL,
            AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
        );
        submit(value);
        vm.prank(AUTHORITY_SUBMITTER);
        vm.expectPartialRevert(AuthorityEventRegistry.AssertionAlreadyRegistered.selector);
        events.recordAssertion(value);
    }

    function testRejectsInvalidPredecessor() public {
        original();
        AuthorityEventRegistry.AuthorityAssertion memory value = assertion(
            E2,
            SERIES,
            bytes32("STALE"),
            E1,
            AuthorityEventRegistry.AssertionKind.CORRECTION,
            AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
        );
        vm.prank(AUTHORITY_SUBMITTER);
        vm.expectPartialRevert(AuthorityEventRegistry.StalePredecessor.selector);
        events.recordAssertion(value);
    }

    function testRejectsTargetFromAnotherSeries() public {
        original();
        submit(
            assertion(
                bytes32("OTHER-EVENT"),
                OTHER_SERIES,
                bytes32(0),
                bytes32(0),
                AuthorityEventRegistry.AssertionKind.ORIGINAL,
                AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
            )
        );
        AuthorityEventRegistry.AuthorityAssertion memory value = assertion(
            E2,
            SERIES,
            E1,
            bytes32("OTHER-EVENT"),
            AuthorityEventRegistry.AssertionKind.CORRECTION,
            AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
        );
        vm.prank(AUTHORITY_SUBMITTER);
        vm.expectPartialRevert(AuthorityEventRegistry.InvalidTarget.selector);
        events.recordAssertion(value);
    }

    function testRejectsWrongKindEffectRelationship() public {
        original();
        AuthorityEventRegistry.AuthorityAssertion memory value = assertion(
            E2,
            SERIES,
            E1,
            E1,
            AuthorityEventRegistry.AssertionKind.REINSTATEMENT,
            AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
        );
        vm.prank(AUTHORITY_SUBMITTER);
        vm.expectPartialRevert(AuthorityEventRegistry.InvalidAssertionEffect.selector);
        events.recordAssertion(value);
    }

    function testRejectsAssertionAfterLiftedHead() public {
        original();
        submit(
            assertion(
                E2,
                SERIES,
                E1,
                E1,
                AuthorityEventRegistry.AssertionKind.REINSTATEMENT,
                AuthorityEventRegistry.AuthorityEffect.EXCLUSION_LIFTED
            )
        );
        AuthorityEventRegistry.AuthorityAssertion memory value = assertion(
            E3,
            SERIES,
            E2,
            E1,
            AuthorityEventRegistry.AssertionKind.CORRECTION,
            AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
        );
        vm.prank(AUTHORITY_SUBMITTER);
        vm.expectPartialRevert(AuthorityEventRegistry.AssertionAfterLiftedHead.selector);
        events.recordAssertion(value);
    }

    function testRejectsUnauthorizedAuthoritySubmission() public {
        AuthorityEventRegistry.AuthorityAssertion memory value = assertion(
            E1,
            SERIES,
            bytes32(0),
            bytes32(0),
            AuthorityEventRegistry.AssertionKind.ORIGINAL,
            AuthorityEventRegistry.AuthorityEffect.EXCLUSION_ACTIVE
        );
        vm.prank(address(0xBAD));
        vm.expectPartialRevert(AuthorityEventRegistry.UnauthorizedAuthoritySubmitter.selector);
        events.recordAssertion(value);
    }
}
