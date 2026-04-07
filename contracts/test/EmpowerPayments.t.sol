// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test, console} from "forge-std/Test.sol";
import {EmpowerPayments} from "../src/EmpowerPayments.sol";

contract EmpowerPaymentsTest is Test {
    EmpowerPayments public pay;
    address public owner = address(this);
    address public treasury = address(0x7EA5);
    address public provider = address(0x9901);
    address public payer = address(0xA7E0);

    bytes32 public bookingId = keccak256("booking-001");

    function setUp() public {
        pay = new EmpowerPayments(owner, treasury);
        vm.deal(payer, 100 ether);
    }

    function test_CreatePayment() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        (address _payer, address _provider, uint256 _amount, EmpowerPayments.Status _status) = pay.payments(bookingId);
        assertEq(_payer, payer);
        assertEq(_provider, provider);
        assertEq(_amount, 1 ether);
        assertEq(uint8(_status), uint8(EmpowerPayments.Status.Pending));
        assertEq(address(pay).balance, 1 ether);
    }

    function test_ReleasePayment() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        uint256 providerBalBefore = provider.balance;
        uint256 treasuryBalBefore = treasury.balance;

        pay.releasePayment(bookingId);

        // 5% fee = 0.05 ether, provider gets 0.95 ether
        assertEq(provider.balance - providerBalBefore, 0.95 ether);
        assertEq(treasury.balance - treasuryBalBefore, 0.05 ether);

        (, , , EmpowerPayments.Status status) = pay.payments(bookingId);
        assertEq(uint8(status), uint8(EmpowerPayments.Status.Released));
    }

    function test_RefundPayment() public {
        vm.prank(payer);
        pay.createPayment{value: 2 ether}(bookingId, provider);

        uint256 payerBalBefore = payer.balance;

        pay.refundPayment(bookingId);

        assertEq(payer.balance - payerBalBefore, 2 ether);

        (, , , EmpowerPayments.Status status) = pay.payments(bookingId);
        assertEq(uint8(status), uint8(EmpowerPayments.Status.Refunded));
    }

    function test_CannotReleaseNonPending() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);
        pay.releasePayment(bookingId);

        vm.expectRevert(abi.encodeWithSelector(EmpowerPayments.PaymentNotPending.selector, bookingId));
        pay.releasePayment(bookingId);
    }

    function test_CannotRefundNonPending() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);
        pay.refundPayment(bookingId);

        vm.expectRevert(abi.encodeWithSelector(EmpowerPayments.PaymentNotPending.selector, bookingId));
        pay.refundPayment(bookingId);
    }

    function test_CannotCreateDuplicate() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        vm.prank(payer);
        vm.expectRevert(abi.encodeWithSelector(EmpowerPayments.PaymentAlreadyExists.selector, bookingId));
        pay.createPayment{value: 1 ether}(bookingId, provider);
    }

    function test_CannotCreateZeroAmount() public {
        vm.prank(payer);
        vm.expectRevert(EmpowerPayments.ZeroAmount.selector);
        pay.createPayment{value: 0}(bookingId, provider);
    }

    function test_CannotCreateZeroProvider() public {
        vm.prank(payer);
        vm.expectRevert(EmpowerPayments.ZeroAddress.selector);
        pay.createPayment{value: 1 ether}(bookingId, address(0));
    }

    function test_OnlyOwnerCanRelease() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        vm.prank(payer);
        vm.expectRevert();
        pay.releasePayment(bookingId);
    }

    function test_OnlyOwnerCanRefund() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        vm.prank(payer);
        vm.expectRevert();
        pay.refundPayment(bookingId);
    }

    function test_EventPaymentCreated() public {
        vm.prank(payer);
        vm.expectEmit(true, true, true, true);
        emit EmpowerPayments.PaymentCreated(bookingId, payer, provider, 1 ether);
        pay.createPayment{value: 1 ether}(bookingId, provider);
    }

    function test_EventPaymentReleased() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        vm.expectEmit(true, true, false, true);
        emit EmpowerPayments.PaymentReleased(bookingId, provider, 0.95 ether, 0.05 ether);
        pay.releasePayment(bookingId);
    }

    function test_EventPaymentRefunded() public {
        vm.prank(payer);
        pay.createPayment{value: 1 ether}(bookingId, provider);

        vm.expectEmit(true, true, false, true);
        emit EmpowerPayments.PaymentRefunded(bookingId, payer, 1 ether);
        pay.refundPayment(bookingId);
    }
}
