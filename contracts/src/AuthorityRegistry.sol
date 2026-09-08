// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

contract AuthorityRegistry {
    enum AuthorityType {
        INVALID,
        EXCLUSION_AUTHORITY
    }

    struct Authority {
        bytes32 authorityId;
        AuthorityType authorityType;
        bool active;
        uint64 registeredAt;
        address authorizedSubmitter;
    }

    error UnauthorizedAdministrator(address caller);
    error InvalidAuthorityId();
    error UnsupportedAuthorityType(uint8 authorityType);
    error InvalidSubmitter();
    error AuthorityAlreadyRegistered(bytes32 authorityId);
    error SubmitterAlreadyAssigned(address submitter);
    error AuthorityNotRegistered(bytes32 authorityId);

    event AuthorityRegistered(
        bytes32 indexed authorityId,
        AuthorityType authorityType,
        address indexed authorizedSubmitter,
        uint64 registeredAt
    );
    event AuthorityActivationChanged(bytes32 indexed authorityId, bool active);

    address public immutable administrator;
    mapping(bytes32 => Authority) private authorities;
    mapping(address => bytes32) private authorityBySubmitter;

    modifier onlyAdministrator() {
        if (msg.sender != administrator) revert UnauthorizedAdministrator(msg.sender);
        _;
    }

    constructor(address administrator_) {
        if (administrator_ == address(0)) revert InvalidSubmitter();
        administrator = administrator_;
    }

    function registerAuthority(
        bytes32 authorityId,
        AuthorityType authorityType,
        address authorizedSubmitter
    ) external onlyAdministrator {
        if (authorityId == bytes32(0)) revert InvalidAuthorityId();
        if (authorityType != AuthorityType.EXCLUSION_AUTHORITY) {
            revert UnsupportedAuthorityType(uint8(authorityType));
        }
        if (authorizedSubmitter == address(0)) revert InvalidSubmitter();
        if (authorities[authorityId].registeredAt != 0) {
            revert AuthorityAlreadyRegistered(authorityId);
        }
        if (authorityBySubmitter[authorizedSubmitter] != bytes32(0)) {
            revert SubmitterAlreadyAssigned(authorizedSubmitter);
        }

        uint64 recordedAt = uint64(block.timestamp);
        authorities[authorityId] = Authority({
            authorityId: authorityId,
            authorityType: authorityType,
            active: true,
            registeredAt: recordedAt,
            authorizedSubmitter: authorizedSubmitter
        });
        authorityBySubmitter[authorizedSubmitter] = authorityId;
        emit AuthorityRegistered(authorityId, authorityType, authorizedSubmitter, recordedAt);
    }

    function setAuthorityActive(bytes32 authorityId, bool active) external onlyAdministrator {
        Authority storage authority = authorities[authorityId];
        if (authority.registeredAt == 0) revert AuthorityNotRegistered(authorityId);
        authority.active = active;
        emit AuthorityActivationChanged(authorityId, active);
    }

    function getAuthority(bytes32 authorityId) external view returns (Authority memory) {
        Authority memory authority = authorities[authorityId];
        if (authority.registeredAt == 0) revert AuthorityNotRegistered(authorityId);
        return authority;
    }

    function authorityIdForSubmitter(address submitter) external view returns (bytes32) {
        return authorityBySubmitter[submitter];
    }

    function isAuthorizedSubmitter(bytes32 authorityId, address submitter)
        external
        view
        returns (bool)
    {
        Authority storage authority = authorities[authorityId];
        return authority.active && authority.authorizedSubmitter == submitter;
    }
}
