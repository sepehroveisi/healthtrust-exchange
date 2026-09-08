// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { OrganizationRegistry } from "./OrganizationRegistry.sol";
import { AuthorityEventRegistry } from "./AuthorityEventRegistry.sol";

contract ResponseLedger {
    enum ResponseState {
        INVALID,
        RECEIVED,
        UNDER_REVIEW,
        DECIDED,
        ACTION_COMPLETED
    }

    struct ResponseVersion {
        bytes32 responseId;
        bytes32 responseVersionId;
        bytes32 previousResponseVersionId;
        bytes32 eventId;
        bytes32 organizationId;
        ResponseState responseState;
        int64 receiptTimestamp;
        bytes32 policyVersionHash;
        bytes32 decisionCommitment;
        bytes32 actionCommitment;
        uint64 recordedAt;
    }

    error InvalidIdentifier();
    error EventNotRegistered(bytes32 eventId);
    error UnauthorizedOrganizationSubmitter(bytes32 organizationId, address caller);
    error ResponseVersionAlreadyRegistered(bytes32 responseVersionId);
    error ResponseStreamAlreadyExists(bytes32 eventId, bytes32 organizationId);
    error ResponseStreamNotFound(bytes32 eventId, bytes32 organizationId);
    error ResponseIdMismatch(bytes32 expected, bytes32 supplied);
    error WrongPreviousVersion(bytes32 expected, bytes32 supplied);
    error InvalidTransition(uint8 previousState, uint8 newState);
    error ResponseIdAlreadyAssigned(bytes32 responseId);
    error InvalidSnapshot();
    error ResponseVersionNotFound(bytes32 responseVersionId);

    event ResponseVersionRecorded(
        bytes32 indexed responseVersionId,
        bytes32 indexed eventId,
        bytes32 indexed organizationId,
        bytes32 responseId,
        ResponseState responseState,
        bytes32 previousResponseVersionId,
        int64 receiptTimestamp,
        bytes32 policyVersionHash,
        bytes32 decisionCommitment,
        bytes32 actionCommitment,
        uint64 recordedAt
    );

    OrganizationRegistry public immutable organizationRegistry;
    AuthorityEventRegistry public immutable authorityEventRegistry;
    mapping(bytes32 => ResponseVersion) private responseVersions;
    mapping(bytes32 => bytes32) private latestVersionByStream;
    mapping(bytes32 => bytes32) private responseIdByStream;
    mapping(bytes32 => bytes32) private streamByResponseId;
    mapping(bytes32 => bytes32[]) private historyByStream;

    constructor(
        OrganizationRegistry organizationRegistry_,
        AuthorityEventRegistry authorityEventRegistry_
    ) {
        organizationRegistry = organizationRegistry_;
        authorityEventRegistry = authorityEventRegistry_;
    }

    function recordResponseVersion(ResponseVersion calldata version) external {
        if (
            version.responseId == bytes32(0) || version.responseVersionId == bytes32(0)
                || version.eventId == bytes32(0) || version.organizationId == bytes32(0)
        ) revert InvalidIdentifier();
        if (!authorityEventRegistry.assertionExists(version.eventId)) {
            revert EventNotRegistered(version.eventId);
        }
        if (!organizationRegistry.isAuthorizedSubmitter(version.organizationId, msg.sender)) {
            revert UnauthorizedOrganizationSubmitter(version.organizationId, msg.sender);
        }
        if (responseVersions[version.responseVersionId].recordedAt != 0) {
            revert ResponseVersionAlreadyRegistered(version.responseVersionId);
        }

        bytes32 streamKey = keccak256(abi.encode(version.eventId, version.organizationId));
        bytes32 currentVersionId = latestVersionByStream[streamKey];
        if (version.responseState == ResponseState.RECEIVED) {
            if (currentVersionId != bytes32(0)) {
                revert ResponseStreamAlreadyExists(version.eventId, version.organizationId);
            }
            if (
                version.previousResponseVersionId != bytes32(0)
                    || version.policyVersionHash != bytes32(0)
                    || version.decisionCommitment != bytes32(0)
                    || version.actionCommitment != bytes32(0)
            ) revert InvalidSnapshot();
            responseIdByStream[streamKey] = version.responseId;
            if (streamByResponseId[version.responseId] != bytes32(0)) {
                revert ResponseIdAlreadyAssigned(version.responseId);
            }
            streamByResponseId[version.responseId] = streamKey;
        } else {
            _validateSuccessor(version, streamKey, currentVersionId);
        }

        uint64 recordedAt = uint64(block.timestamp);
        responseVersions[version.responseVersionId] = ResponseVersion({
            responseId: version.responseId,
            responseVersionId: version.responseVersionId,
            previousResponseVersionId: version.previousResponseVersionId,
            eventId: version.eventId,
            organizationId: version.organizationId,
            responseState: version.responseState,
            receiptTimestamp: version.receiptTimestamp,
            policyVersionHash: version.policyVersionHash,
            decisionCommitment: version.decisionCommitment,
            actionCommitment: version.actionCommitment,
            recordedAt: recordedAt
        });
        latestVersionByStream[streamKey] = version.responseVersionId;
        historyByStream[streamKey].push(version.responseVersionId);

        emit ResponseVersionRecorded(
            version.responseVersionId,
            version.eventId,
            version.organizationId,
            version.responseId,
            version.responseState,
            version.previousResponseVersionId,
            version.receiptTimestamp,
            version.policyVersionHash,
            version.decisionCommitment,
            version.actionCommitment,
            recordedAt
        );
    }

    function _validateSuccessor(
        ResponseVersion calldata version,
        bytes32 streamKey,
        bytes32 currentVersionId
    ) private view {
        if (currentVersionId == bytes32(0)) {
            revert ResponseStreamNotFound(version.eventId, version.organizationId);
        }
        if (responseIdByStream[streamKey] != version.responseId) {
            revert ResponseIdMismatch(responseIdByStream[streamKey], version.responseId);
        }
        if (version.previousResponseVersionId != currentVersionId) {
            revert WrongPreviousVersion(currentVersionId, version.previousResponseVersionId);
        }
        ResponseVersion storage previous = responseVersions[currentVersionId];
        if (uint8(version.responseState) != uint8(previous.responseState) + 1) {
            revert InvalidTransition(uint8(previous.responseState), uint8(version.responseState));
        }
        if (version.receiptTimestamp != previous.receiptTimestamp) revert InvalidSnapshot();

        if (version.responseState == ResponseState.UNDER_REVIEW) {
            if (
                version.policyVersionHash == bytes32(0) || version.decisionCommitment != bytes32(0)
                    || version.actionCommitment != bytes32(0)
            ) revert InvalidSnapshot();
        } else if (version.responseState == ResponseState.DECIDED) {
            if (
                version.policyVersionHash != previous.policyVersionHash
                    || version.decisionCommitment == bytes32(0)
                    || version.actionCommitment != bytes32(0)
            ) revert InvalidSnapshot();
        } else if (version.responseState == ResponseState.ACTION_COMPLETED) {
            if (
                version.policyVersionHash != previous.policyVersionHash
                    || version.decisionCommitment != previous.decisionCommitment
                    || version.actionCommitment == bytes32(0)
            ) revert InvalidSnapshot();
        } else {
            revert InvalidTransition(uint8(previous.responseState), uint8(version.responseState));
        }
    }

    function getResponseVersion(bytes32 responseVersionId)
        external
        view
        returns (ResponseVersion memory)
    {
        ResponseVersion memory version = responseVersions[responseVersionId];
        if (version.recordedAt == 0) revert ResponseVersionNotFound(responseVersionId);
        return version;
    }

    function latestResponseVersionId(bytes32 eventId, bytes32 organizationId)
        external
        view
        returns (bytes32)
    {
        return latestVersionByStream[keccak256(abi.encode(eventId, organizationId))];
    }

    function historyLength(bytes32 eventId, bytes32 organizationId)
        external
        view
        returns (uint256)
    {
        return historyByStream[keccak256(abi.encode(eventId, organizationId))].length;
    }

    function historyResponseVersionId(bytes32 eventId, bytes32 organizationId, uint256 index)
        external
        view
        returns (bytes32)
    {
        return historyByStream[keccak256(abi.encode(eventId, organizationId))][index];
    }
}
