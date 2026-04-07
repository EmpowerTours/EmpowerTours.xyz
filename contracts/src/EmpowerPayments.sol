// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Ownable2Step, Ownable} from "@openzeppelin/contracts/access/Ownable2Step.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title EmpowerPayments — Escrow payments for EmpowerTours bookings
/// @notice Accepts native MON, holds in escrow, releases to provider minus 5% fee
contract EmpowerPayments is Ownable2Step, ReentrancyGuard {
    enum Status {
        Pending,
        Released,
        Refunded
    }

    struct Payment {
        address payer;
        address provider;
        uint256 amount;
        Status status;
    }

    /// @notice Treasury address that receives the 5% fee
    address public treasury;

    /// @notice Fee basis points (500 = 5%)
    uint256 public constant FEE_BPS = 500;
    uint256 public constant BPS_DENOMINATOR = 10_000;

    /// @dev bookingId => Payment
    mapping(bytes32 => Payment) public payments;

    event PaymentCreated(bytes32 indexed bookingId, address indexed payer, address indexed provider, uint256 amount);
    event PaymentReleased(bytes32 indexed bookingId, address indexed provider, uint256 providerAmount, uint256 fee);
    event PaymentRefunded(bytes32 indexed bookingId, address indexed payer, uint256 amount);

    error PaymentAlreadyExists(bytes32 bookingId);
    error PaymentNotPending(bytes32 bookingId);
    error ZeroAmount();
    error ZeroAddress();
    error TransferFailed();

    constructor(address initialOwner, address _treasury) Ownable(initialOwner) {
        if (_treasury == address(0)) revert ZeroAddress();
        treasury = _treasury;
    }

    /// @notice Create an escrow payment for a booking
    /// @param bookingId Unique identifier for the booking
    /// @param provider Service provider who will receive the funds
    function createPayment(bytes32 bookingId, address provider) external payable {
        if (msg.value == 0) revert ZeroAmount();
        if (provider == address(0)) revert ZeroAddress();
        if (payments[bookingId].amount != 0) revert PaymentAlreadyExists(bookingId);

        payments[bookingId] = Payment({
            payer: msg.sender,
            provider: provider,
            amount: msg.value,
            status: Status.Pending
        });

        emit PaymentCreated(bookingId, msg.sender, provider, msg.value);
    }

    /// @notice Release escrowed funds to the provider minus 5% fee
    /// @param bookingId The booking to release payment for
    function releasePayment(bytes32 bookingId) external onlyOwner nonReentrant {
        Payment storage p = payments[bookingId];
        if (p.status != Status.Pending) revert PaymentNotPending(bookingId);

        p.status = Status.Released;

        uint256 fee = (p.amount * FEE_BPS) / BPS_DENOMINATOR;
        uint256 providerAmount = p.amount - fee;

        (bool sentProvider,) = p.provider.call{value: providerAmount}("");
        if (!sentProvider) revert TransferFailed();

        (bool sentTreasury,) = treasury.call{value: fee}("");
        if (!sentTreasury) revert TransferFailed();

        emit PaymentReleased(bookingId, p.provider, providerAmount, fee);
    }

    /// @notice Refund escrowed funds back to the payer
    /// @param bookingId The booking to refund
    function refundPayment(bytes32 bookingId) external onlyOwner nonReentrant {
        Payment storage p = payments[bookingId];
        if (p.status != Status.Pending) revert PaymentNotPending(bookingId);

        p.status = Status.Refunded;

        (bool sent,) = p.payer.call{value: p.amount}("");
        if (!sent) revert TransferFailed();

        emit PaymentRefunded(bookingId, p.payer, p.amount);
    }

    /// @notice Update the treasury address
    /// @param _treasury New treasury address
    function setTreasury(address _treasury) external onlyOwner {
        if (_treasury == address(0)) revert ZeroAddress();
        treasury = _treasury;
    }
}
