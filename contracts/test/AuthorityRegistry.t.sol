// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { TestBase } from "./TestBase.sol";
import { AuthorityRegistry } from "../src/AuthorityRegistry.sol";

contract AuthorityRegistryTest is TestBase {
    AuthorityRegistry private registry;
    bytes32 private constant AUTHORITY = bytes32("HHS-OIG-DEMO");
    address private constant SUBMITTER = address(0xA17);

    function setUp() public {
        registry = new AuthorityRegistry(address(this));
    }

    function testRegistersAndExposesAuthority() public {
        registry.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, SUBMITTER
        );
        AuthorityRegistry.Authority memory authority = registry.getAuthority(AUTHORITY);
        assertEq(authority.authorityId, AUTHORITY);
        assertEq(uint256(authority.authorityType), 1);
        assertTrue(authority.active);
        assertEq(authority.authorizedSubmitter, SUBMITTER);
        assertTrue(registry.isAuthorizedSubmitter(AUTHORITY, SUBMITTER));
    }

    function testRejectsDuplicateAuthority() public {
        registry.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, SUBMITTER
        );
        vm.expectPartialRevert(AuthorityRegistry.AuthorityAlreadyRegistered.selector);
        registry.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, address(0xB)
        );
    }

    function testRejectsUnauthorizedRegistration() public {
        vm.prank(address(0xBAD));
        vm.expectPartialRevert(AuthorityRegistry.UnauthorizedAdministrator.selector);
        registry.registerAuthority(
            AUTHORITY, AuthorityRegistry.AuthorityType.EXCLUSION_AUTHORITY, SUBMITTER
        );
    }

    function testRejectsUnsupportedAuthorityType() public {
        vm.expectPartialRevert(AuthorityRegistry.UnsupportedAuthorityType.selector);
        registry.registerAuthority(AUTHORITY, AuthorityRegistry.AuthorityType.INVALID, SUBMITTER);
    }
}
