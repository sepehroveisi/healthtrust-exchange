// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

import { OrganizationRegistry } from "../src/OrganizationRegistry.sol";
import { AuthorityRegistry } from "../src/AuthorityRegistry.sol";
import { AuthorityEventRegistry } from "../src/AuthorityEventRegistry.sol";
import { ResponseLedger } from "../src/ResponseLedger.sol";

interface IDeploymentVm {
    function envUint(string calldata name) external returns (uint256 value);
    function addr(uint256 privateKey) external returns (address keyAddr);
    function startBroadcast(uint256 privateKey) external;
    function stopBroadcast() external;
}

contract DeployAuthorityLedger {
    IDeploymentVm private constant VM =
        IDeploymentVm(address(uint160(uint256(keccak256("hevm cheat code")))));

    bytes32 private constant HOSPITAL = bytes32("HOSPITAL-A");
    bytes32 private constant PAYER = bytes32("PAYER-B");
    bytes32 private constant STAFFING = bytes32("STAFFING-AGENCY-C");
    bytes32 private constant AUTHORITY = bytes32("HHS-OIG-DEMO");

    address private constant AUTHORITY_SUBMITTER = 0xe05fcC23807536bEe418f142D19fa0d21BB0cfF7;
    address private constant HOSPITAL_SUBMITTER = 0x0376AAc07Ad725E01357B1725B5ceC61aE10473c;
    address private constant PAYER_SUBMITTER = 0xb040e0fAac56886b0f29Af446544AED0A154ED29;
    address private constant STAFFING_SUBMITTER = 0xF5A5E415061470A8b9137959180901aEa72450a4;

    function run()
        external
        returns (
            OrganizationRegistry organizations,
            AuthorityRegistry authorities,
            AuthorityEventRegistry authorityEvents,
            ResponseLedger responses
        )
    {
        uint256 deployerKey = VM.envUint("DEMO_DEPLOYER_PRIVATE_KEY");
        address deployer = VM.addr(deployerKey);

        VM.startBroadcast(deployerKey);
        organizations = new OrganizationRegistry(deployer);
        authorities = new AuthorityRegistry(deployer);
        authorityEvents = new AuthorityEventRegistry(authorities);
        responses = new ResponseLedger(organizations, authorityEvents);

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
        VM.stopBroadcast();
    }
}
