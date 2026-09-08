// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { AuthorityRegistry } from "./AuthorityRegistry.sol";

contract AuthorityEventRegistry {
    enum EventType {
        INVALID,
        EXCLUSION
    }

    enum AssertionKind {
        INVALID,
        ORIGINAL,
        CORRECTION,
        SUPERSESSION,
        REINSTATEMENT
    }

    enum AuthorityEffect {
        INVALID,
        EXCLUSION_ACTIVE,
        EXCLUSION_LIFTED
    }

    struct AuthorityAssertion {
        bytes32 eventId;
        bytes32 eventSeriesId;
        bytes32 previousEventId;
        bytes32 targetEventId;
        bytes32 authorityId;
        bytes32 eventCommitment;
        EventType eventType;
        AssertionKind assertionKind;
        AuthorityEffect authorityEffect;
        int64 effectiveTime;
        uint64 recordedAt;
    }

    error InvalidIdentifier();
    error InvalidCommitment();
    error AssertionAlreadyRegistered(bytes32 eventId);
    error UnauthorizedAuthoritySubmitter(bytes32 authorityId, address caller);
    error UnsupportedEventType(uint8 eventType);
    error InvalidOriginalLineage();
    error SeriesAlreadyExists(bytes32 eventSeriesId);
    error SeriesNotFound(bytes32 eventSeriesId);
    error StalePredecessor(bytes32 expected, bytes32 supplied);
    error InvalidAssertionKind(uint8 assertionKind);
    error InvalidAssertionEffect(uint8 assertionKind, uint8 authorityEffect);
    error InvalidTarget(bytes32 targetEventId);
    error AuthorityMismatch(bytes32 expected, bytes32 supplied);
    error AssertionAfterLiftedHead(bytes32 eventSeriesId);
    error AssertionNotFound(bytes32 eventId);

    event AuthorityAssertionRecorded(
        bytes32 indexed eventId,
        bytes32 indexed eventSeriesId,
        bytes32 indexed authorityId,
        bytes32 eventCommitment,
        EventType eventType,
        AssertionKind assertionKind,
        AuthorityEffect authorityEffect,
        bytes32 previousEventId,
        bytes32 targetEventId,
        int64 effectiveTime,
        uint64 recordedAt
    );

    AuthorityRegistry public immutable authorityRegistry;
    mapping(bytes32 => AuthorityAssertion) private assertions;
    mapping(bytes32 => bytes32) private currentHeadBySeries;
    mapping(bytes32 => bytes32[]) private historyBySeries;

    constructor(AuthorityRegistry authorityRegistry_) {
        authorityRegistry = authorityRegistry_;
    }

    function recordAssertion(AuthorityAssertion calldata assertion) external {
        if (
            assertion.eventId == bytes32(0) || assertion.eventSeriesId == bytes32(0)
                || assertion.authorityId == bytes32(0)
        ) revert InvalidIdentifier();
        if (assertion.eventCommitment == bytes32(0)) revert InvalidCommitment();
        if (assertions[assertion.eventId].recordedAt != 0) {
            revert AssertionAlreadyRegistered(assertion.eventId);
        }
        if (!authorityRegistry.isAuthorizedSubmitter(assertion.authorityId, msg.sender)) {
            revert UnauthorizedAuthoritySubmitter(assertion.authorityId, msg.sender);
        }
        if (assertion.eventType != EventType.EXCLUSION) {
            revert UnsupportedEventType(uint8(assertion.eventType));
        }

        bytes32 head = currentHeadBySeries[assertion.eventSeriesId];
        if (assertion.assertionKind == AssertionKind.ORIGINAL) {
            if (head != bytes32(0)) revert SeriesAlreadyExists(assertion.eventSeriesId);
            if (
                assertion.previousEventId != bytes32(0) || assertion.targetEventId != bytes32(0)
                    || assertion.authorityEffect != AuthorityEffect.EXCLUSION_ACTIVE
            ) revert InvalidOriginalLineage();
        } else {
            _validateSuccessor(assertion, head);
        }

        uint64 recordedAt = uint64(block.timestamp);
        assertions[assertion.eventId] = AuthorityAssertion({
            eventId: assertion.eventId,
            eventSeriesId: assertion.eventSeriesId,
            previousEventId: assertion.previousEventId,
            targetEventId: assertion.targetEventId,
            authorityId: assertion.authorityId,
            eventCommitment: assertion.eventCommitment,
            eventType: assertion.eventType,
            assertionKind: assertion.assertionKind,
            authorityEffect: assertion.authorityEffect,
            effectiveTime: assertion.effectiveTime,
            recordedAt: recordedAt
        });
        currentHeadBySeries[assertion.eventSeriesId] = assertion.eventId;
        historyBySeries[assertion.eventSeriesId].push(assertion.eventId);

        emit AuthorityAssertionRecorded(
            assertion.eventId,
            assertion.eventSeriesId,
            assertion.authorityId,
            assertion.eventCommitment,
            assertion.eventType,
            assertion.assertionKind,
            assertion.authorityEffect,
            assertion.previousEventId,
            assertion.targetEventId,
            assertion.effectiveTime,
            recordedAt
        );
    }

    function _validateSuccessor(AuthorityAssertion calldata assertion, bytes32 head) private view {
        if (head == bytes32(0)) revert SeriesNotFound(assertion.eventSeriesId);
        if (assertion.previousEventId != head) {
            revert StalePredecessor(head, assertion.previousEventId);
        }
        AuthorityAssertion storage predecessor = assertions[head];
        if (predecessor.authorityId != assertion.authorityId) {
            revert AuthorityMismatch(predecessor.authorityId, assertion.authorityId);
        }
        if (predecessor.authorityEffect == AuthorityEffect.EXCLUSION_LIFTED) {
            revert AssertionAfterLiftedHead(assertion.eventSeriesId);
        }
        if (
            assertion.assertionKind < AssertionKind.CORRECTION
                || assertion.assertionKind > AssertionKind.REINSTATEMENT
        ) revert InvalidAssertionKind(uint8(assertion.assertionKind));
        AuthorityAssertion storage target = assertions[assertion.targetEventId];
        if (
            assertion.targetEventId == bytes32(0) || assertion.targetEventId == assertion.eventId
                || target.recordedAt == 0 || target.eventSeriesId != assertion.eventSeriesId
                || target.authorityEffect != AuthorityEffect.EXCLUSION_ACTIVE
        ) revert InvalidTarget(assertion.targetEventId);
        if (assertion.assertionKind == AssertionKind.REINSTATEMENT
                ? assertion.authorityEffect != AuthorityEffect.EXCLUSION_LIFTED
                : assertion.authorityEffect != AuthorityEffect.EXCLUSION_ACTIVE) {
            revert InvalidAssertionEffect(
                uint8(assertion.assertionKind), uint8(assertion.authorityEffect)
            );
        }
    }

    function assertionExists(bytes32 eventId) external view returns (bool) {
        return assertions[eventId].recordedAt != 0;
    }

    function getAssertion(bytes32 eventId) external view returns (AuthorityAssertion memory) {
        AuthorityAssertion memory assertion = assertions[eventId];
        if (assertion.recordedAt == 0) revert AssertionNotFound(eventId);
        return assertion;
    }

    function currentHead(bytes32 eventSeriesId) external view returns (bytes32) {
        return currentHeadBySeries[eventSeriesId];
    }

    function historyLength(bytes32 eventSeriesId) external view returns (uint256) {
        return historyBySeries[eventSeriesId].length;
    }

    function historyEventId(bytes32 eventSeriesId, uint256 index) external view returns (bytes32) {
        return historyBySeries[eventSeriesId][index];
    }
}
