// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test, console} from "forge-std/Test.sol";
import {EmpowerMembership} from "../src/MembershipNFT.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

// Minimal ERC20 mock for WMON/TOURS
contract MockERC20 is IERC20 {
    string public name;
    string public symbol;
    uint8 public decimals = 18;
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    constructor(string memory _name, string memory _symbol) {
        name = _name;
        symbol = _symbol;
    }

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
        totalSupply += amount;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
        emit Transfer(msg.sender, to, amount);
        return true;
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        allowance[from][msg.sender] -= amount;
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
        return true;
    }
}

contract EmpowerMembershipTest is Test {
    EmpowerMembership public membership;
    MockERC20 public wmon;
    MockERC20 public tours;

    address public owner = address(this);
    address public treasury = address(0x7EA5);
    address public alice = address(0xA11CE);
    address public bob = address(0xB0B);

    function setUp() public {
        wmon = new MockERC20("Wrapped MON", "WMON");
        tours = new MockERC20("TOURS", "TOURS");
        membership = new EmpowerMembership(address(wmon), address(tours), treasury);

        // Fund alice and bob with WMON
        wmon.mint(alice, 1000 ether);
        wmon.mint(bob, 1000 ether);

        // Set tier prices
        membership.setTierPrices(10 ether, 25 ether, 50 ether);

        // Start a 90-day cohort
        membership.startCohort(90 days);
    }

    // ─── Application Tests ───────────────────────────────────────────────────────

    function test_SubmitApplication() public {
        vm.prank(alice);
        membership.applyForMembership(
            EmpowerMembership.Tier.Explorer,
            "Alice",
            "I love Guerrero"
        );

        EmpowerMembership.Application memory app = membership.getApplication(1);
        assertEq(app.applicant, alice);
        assertEq(uint8(app.requestedTier), uint8(EmpowerMembership.Tier.Explorer));
        assertEq(uint8(app.status), uint8(EmpowerMembership.AppStatus.Pending));
        assertEq(membership.totalApplications(), 1);
    }

    function test_ApproveApplication() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Builder, "Alice", "reason");

        membership.approveApplication(1);

        EmpowerMembership.Application memory app = membership.getApplication(1);
        assertEq(uint8(app.status), uint8(EmpowerMembership.AppStatus.Approved));

        // Should be whitelisted
        assertTrue(membership.whitelisted(1, alice));
    }

    function test_RejectApplication() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");

        membership.rejectApplication(1);

        EmpowerMembership.Application memory app = membership.getApplication(1);
        assertEq(uint8(app.status), uint8(EmpowerMembership.AppStatus.Rejected));
    }

    function test_ReapplyAfterRejection() public {
        vm.startPrank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "first try");
        vm.stopPrank();

        membership.rejectApplication(1);

        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Builder, "Alice", "second try");

        assertEq(membership.totalApplications(), 2);
    }

    function test_CannotDoubleApply() public {
        vm.startPrank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");

        vm.expectRevert("already applied");
        membership.applyForMembership(EmpowerMembership.Tier.Builder, "Alice", "again");
        vm.stopPrank();
    }

    function test_BatchApprove() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        vm.prank(bob);
        membership.applyForMembership(EmpowerMembership.Tier.Builder, "Bob", "reason");

        uint256[] memory ids = new uint256[](2);
        ids[0] = 1;
        ids[1] = 2;
        membership.batchApproveApplications(ids);

        assertTrue(membership.whitelisted(1, alice));
        assertTrue(membership.whitelisted(1, bob));
    }

    // ─── Payment & Membership Tests ──────────────────────────────────────────────

    function test_PayMonthlyMintsMembership() public {
        // Apply and approve alice
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        // Alice approves WMON and pays
        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();
        vm.stopPrank();

        // Should have membership NFT
        assertTrue(membership.isMember(alice));
        assertEq(membership.balanceOf(alice), 1);

        // Check member data
        EmpowerMembership.Member memory m = membership.getMember(1, alice);
        assertEq(m.monthsPaid, 1);
        assertEq(m.totalPaid, 10 ether); // Explorer price
        assertEq(uint8(m.tier), uint8(EmpowerMembership.Tier.Explorer));
    }

    function test_TreasuryReceivesFee() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        uint256 treasuryBefore = wmon.balanceOf(treasury);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();
        vm.stopPrank();

        // 5% of 10 ether = 0.5 ether
        assertEq(wmon.balanceOf(treasury) - treasuryBefore, 0.5 ether);
    }

    function test_PoolReceives95Percent() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();
        vm.stopPrank();

        EmpowerMembership.Cohort memory c = membership.getCohort(1);
        assertEq(c.poolBalance, 9.5 ether); // 95% of 10
    }

    function test_CannotPayWithoutWhitelist() public {
        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        vm.expectRevert("not whitelisted");
        membership.payMonthly();
        vm.stopPrank();
    }

    function test_CannotPayTooSoon() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();

        vm.expectRevert("too soon");
        membership.payMonthly();
        vm.stopPrank();
    }

    function test_CanPayAfterInterval() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();

        vm.warp(block.timestamp + 25 days);
        membership.payMonthly();
        vm.stopPrank();

        EmpowerMembership.Member memory m = membership.getMember(1, alice);
        assertEq(m.monthsPaid, 2);
    }

    // ─── Soulbound Tests ─────────────────────────────────────────────────────────

    function test_SoulboundBlocksTransfer() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();

        uint256 tokenId = membership.getMember(1, alice).tokenId;
        vm.expectRevert(EmpowerMembership.SoulboundTransferBlocked.selector);
        membership.transferFrom(alice, bob, tokenId);
        vm.stopPrank();
    }

    // ─── Tier Upgrade Tests ──────────────────────────────────────────────────────

    function test_UpgradeTier() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        membership.upgradeTier(alice, EmpowerMembership.Tier.Founder);

        EmpowerMembership.Member memory m = membership.getMember(1, alice);
        assertEq(uint8(m.tier), uint8(EmpowerMembership.Tier.Founder));
    }

    // ─── Ban Tests ───────────────────────────────────────────────────────────────

    function test_BanMember() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        membership.banMember(1, alice);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        vm.expectRevert("member banned");
        membership.payMonthly();
        vm.stopPrank();
    }

    // ─── Founder Distribution Tests ──────────────────────────────────────────────

    function test_FounderDistribution() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");
        membership.approveApplication(1);

        vm.startPrank(alice);
        wmon.approve(address(membership), 100 ether);
        membership.payMonthly();
        vm.stopPrank();

        // End cohort
        membership.endCohort();

        // Select alice as founder
        address[] memory founders = new address[](1);
        founders[0] = alice;
        uint256[] memory amounts = new uint256[](1);
        amounts[0] = 5 ether;

        uint256 aliceBefore = wmon.balanceOf(alice);
        membership.selectFounders(1, founders, amounts);

        assertEq(wmon.balanceOf(alice) - aliceBefore, 5 ether);

        EmpowerMembership.Member memory m = membership.getMember(1, alice);
        assertTrue(m.isFounder);
        assertEq(m.founderAmount, 5 ether);
    }

    // ─── Airdrop Tests ───────────────────────────────────────────────────────────

    function test_AirdropTours() public {
        tours.mint(address(this), 100 ether);
        tours.approve(address(membership), 100 ether);
        membership.fundTours(50 ether);

        address[] memory recipients = new address[](1);
        recipients[0] = alice;
        uint256[] memory amounts = new uint256[](1);
        amounts[0] = 10 ether;

        membership.airdropTours(recipients, amounts);
        assertEq(tours.balanceOf(alice), 10 ether);
    }

    // ─── Cohort Tests ────────────────────────────────────────────────────────────

    function test_StartNewCohortDeactivatesOld() public {
        // Cohort 1 already active from setUp
        membership.startCohort(180 days);

        EmpowerMembership.Cohort memory c1 = membership.getCohort(1);
        assertFalse(c1.active);
        assertTrue(c1.endTime > 0);

        EmpowerMembership.Cohort memory c2 = membership.getCohort(2);
        assertTrue(c2.active);
        assertEq(membership.currentCohortId(), 2);
    }

    // ─── Rescue Tests ────────────────────────────────────────────────────────────

    function test_RescueTokens() public {
        // Send some random tokens to contract
        tours.mint(address(membership), 10 ether);

        membership.rescueTokens(address(tours), owner, 10 ether);
        assertEq(tours.balanceOf(owner), 10 ether);
    }

    // ─── Access Control Tests ────────────────────────────────────────────────────

    function test_OnlyOwnerCanApprove() public {
        vm.prank(alice);
        membership.applyForMembership(EmpowerMembership.Tier.Explorer, "Alice", "reason");

        vm.prank(alice);
        vm.expectRevert();
        membership.approveApplication(1);
    }

    function test_OnlyOwnerCanSetPrices() public {
        vm.prank(alice);
        vm.expectRevert();
        membership.setTierPrices(1 ether, 2 ether, 3 ether);
    }
}
