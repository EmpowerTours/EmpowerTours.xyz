// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console} from "forge-std/Script.sol";
import {EmpowerMembership} from "../src/MembershipNFT.sol";
import {EmpowerPayments} from "../src/EmpowerPayments.sol";

contract Deploy is Script {
    // Monad mainnet token addresses
    address constant WMON = 0x3bd359C1119dA7Da1D913D1C4D2B7c461115433A;
    address constant TOURS = 0x45b76a127167fD7FC7Ed264ad490144300eCfcBF;

    function run() external {
        address deployer = msg.sender;
        address treasury = vm.envOr("TREASURY", deployer);

        vm.startBroadcast();

        EmpowerMembership membership = new EmpowerMembership(WMON, TOURS, treasury);
        console.log("EmpowerMembership deployed at:", address(membership));

        EmpowerPayments payments = new EmpowerPayments(deployer, treasury);
        console.log("EmpowerPayments deployed at:", address(payments));

        vm.stopBroadcast();
    }
}
