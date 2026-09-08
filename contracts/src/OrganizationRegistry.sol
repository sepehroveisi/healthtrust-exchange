// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

contract OrganizationRegistry {
    enum OrganizationType {
        INVALID,
        HOSPITAL,
        PAYER,
        STAFFING_AGENCY
    }

    struct Organization {
        bytes32 organizationId;
        OrganizationType organizationType;
        bool active;
        uint64 registeredAt;
        address authorizedSubmitter;
    }

    error UnauthorizedAdministrator(address caller);
    error InvalidOrganizationId();
    error UnsupportedOrganizationType(uint8 organizationType);
    error InvalidSubmitter();
    error OrganizationAlreadyRegistered(bytes32 organizationId);
    error SubmitterAlreadyAssigned(address submitter);
    error OrganizationNotRegistered(bytes32 organizationId);

    event OrganizationRegistered(
        bytes32 indexed organizationId,
        OrganizationType organizationType,
        address indexed authorizedSubmitter,
        uint64 registeredAt
    );
    event OrganizationActivationChanged(bytes32 indexed organizationId, bool active);

    address public immutable administrator;
    mapping(bytes32 => Organization) private organizations;
    mapping(address => bytes32) private organizationBySubmitter;

    modifier onlyAdministrator() {
        if (msg.sender != administrator) revert UnauthorizedAdministrator(msg.sender);
        _;
    }

    constructor(address administrator_) {
        if (administrator_ == address(0)) revert InvalidSubmitter();
        administrator = administrator_;
    }

    function registerOrganization(
        bytes32 organizationId,
        OrganizationType organizationType,
        address authorizedSubmitter
    ) external onlyAdministrator {
        if (organizationId == bytes32(0)) {
            revert InvalidOrganizationId();
        }
        if (
            organizationType < OrganizationType.HOSPITAL
                || organizationType > OrganizationType.STAFFING_AGENCY
        ) revert UnsupportedOrganizationType(uint8(organizationType));
        if (authorizedSubmitter == address(0)) revert InvalidSubmitter();
        if (organizations[organizationId].registeredAt != 0) {
            revert OrganizationAlreadyRegistered(organizationId);
        }
        if (organizationBySubmitter[authorizedSubmitter] != bytes32(0)) {
            revert SubmitterAlreadyAssigned(authorizedSubmitter);
        }

        uint64 recordedAt = uint64(block.timestamp);
        organizations[organizationId] = Organization({
            organizationId: organizationId,
            organizationType: organizationType,
            active: true,
            registeredAt: recordedAt,
            authorizedSubmitter: authorizedSubmitter
        });
        organizationBySubmitter[authorizedSubmitter] = organizationId;
        emit OrganizationRegistered(
            organizationId, organizationType, authorizedSubmitter, recordedAt
        );
    }

    function setOrganizationActive(bytes32 organizationId, bool active) external onlyAdministrator {
        Organization storage organization = organizations[organizationId];
        if (organization.registeredAt == 0) revert OrganizationNotRegistered(organizationId);
        organization.active = active;
        emit OrganizationActivationChanged(organizationId, active);
    }

    function getOrganization(bytes32 organizationId) external view returns (Organization memory) {
        Organization memory organization = organizations[organizationId];
        if (organization.registeredAt == 0) revert OrganizationNotRegistered(organizationId);
        return organization;
    }

    function organizationIdForSubmitter(address submitter) external view returns (bytes32) {
        return organizationBySubmitter[submitter];
    }

    function isAuthorizedSubmitter(bytes32 organizationId, address submitter)
        external
        view
        returns (bool)
    {
        Organization storage organization = organizations[organizationId];
        return organization.active && organization.authorizedSubmitter == submitter;
    }
}
