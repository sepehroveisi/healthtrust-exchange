// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { TestBase } from "./TestBase.sol";
import { OrganizationRegistry } from "../src/OrganizationRegistry.sol";

contract OrganizationRegistryTest is TestBase {
    OrganizationRegistry private registry;
    bytes32 private constant HOSPITAL = bytes32("HOSPITAL-A");
    address private constant SUBMITTER = address(0xA11CE);

    function setUp() public {
        registry = new OrganizationRegistry(address(this));
    }

    function testRegistersAndExposesOrganization() public {
        vm.warp(1_700_000_000);
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, SUBMITTER
        );
        OrganizationRegistry.Organization memory organization = registry.getOrganization(HOSPITAL);
        assertEq(organization.organizationId, HOSPITAL);
        assertEq(uint256(organization.organizationType), 1);
        assertTrue(organization.active);
        assertEq(uint256(organization.registeredAt), 1_700_000_000);
        assertEq(organization.authorizedSubmitter, SUBMITTER);
        assertTrue(registry.isAuthorizedSubmitter(HOSPITAL, SUBMITTER));
    }

    function testRejectsDuplicateOrganization() public {
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, SUBMITTER
        );
        vm.expectPartialRevert(OrganizationRegistry.OrganizationAlreadyRegistered.selector);
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, address(0xB0B)
        );
    }

    function testRejectsUnauthorizedRegistration() public {
        vm.prank(address(0xBAD));
        vm.expectPartialRevert(OrganizationRegistry.UnauthorizedAdministrator.selector);
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, SUBMITTER
        );
    }

    function testRejectsUnsupportedTypeAndAcceptsAllRc2Types() public {
        vm.expectPartialRevert(OrganizationRegistry.UnsupportedOrganizationType.selector);
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.INVALID, SUBMITTER
        );
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, address(0xA)
        );
        registry.registerOrganization(
            bytes32("PAYER-B"), OrganizationRegistry.OrganizationType.PAYER, address(0xB)
        );
        registry.registerOrganization(
            bytes32("STAFFING-AGENCY-C"),
            OrganizationRegistry.OrganizationType.STAFFING_AGENCY,
            address(0xC)
        );
    }

    function testAdministratorCanDeactivateOrganization() public {
        registry.registerOrganization(
            HOSPITAL, OrganizationRegistry.OrganizationType.HOSPITAL, SUBMITTER
        );
        registry.setOrganizationActive(HOSPITAL, false);
        assertFalse(registry.getOrganization(HOSPITAL).active);
        assertFalse(registry.isAuthorizedSubmitter(HOSPITAL, SUBMITTER));
    }
}
