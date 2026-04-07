// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ERC721} from "@openzeppelin/contracts/token/ERC721/ERC721.sol";
import {Ownable2Step, Ownable} from "@openzeppelin/contracts/access/Ownable2Step.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

/// @title EmpowerTours Ecosystem Membership
/// @notice Soulbound ERC-721 membership NFT that gates the entire EmpowerTours ecosystem:
///         tourism app, TURBO, Farcaster MiniApp, Monad Odyssey, and all future products.
///         Based on TurboCohortV6 patterns — WMON payments, cohorts, tiers, community pool,
///         founder graduation, TOURS airdrops, on-chain SVG cards.
/// @dev Security: Ownable2Step, ReentrancyGuard, SafeERC20, soulbound via _update override,
///      checks-effects-interactions, batch limits, payment cap.
contract EmpowerMembership is ERC721, Ownable2Step, ReentrancyGuard {
    using SafeERC20 for IERC20;

    // ─── Types ───────────────────────────────────────────────────────────────────

    enum Tier { None, Explorer, Builder, Founder }
    enum AppStatus { Pending, Approved, Rejected }

    struct Cohort {
        uint256 startTime;
        uint256 endTime;
        uint256 duration;
        uint256 poolBalance;
        uint256 totalCollected;
        uint256 memberCount;
        bool active;
        bool finalized;
    }

    struct Member {
        Tier tier;
        uint256 cohortId;
        uint256 monthsPaid;
        uint256 lastPaymentTime;
        uint256 totalPaid;
        bool isFounder;
        uint256 founderAmount;
        uint256 tokenId;
        bool banned;
    }

    struct Application {
        address applicant;
        Tier requestedTier;
        string name;
        string reason;
        AppStatus status;
        uint256 submittedAt;
        uint256 reviewedAt;
    }

    // ─── Constants ───────────────────────────────────────────────────────────────

    uint256 public constant TREASURY_FEE_BPS = 500; // 5%
    uint256 public constant BPS_DENOMINATOR = 10_000;
    uint256 public constant MAX_TIER_PRICE = 2000 ether;
    uint256 public constant MIN_PAYMENT_INTERVAL = 25 days;
    uint256 public constant MAX_BATCH_SIZE = 200;
    uint256 public constant MAX_PAYMENTS = 12;

    // ─── Immutables ──────────────────────────────────────────────────────────────

    IERC20 public immutable wmon;
    IERC20 public immutable tours;

    // ─── State ───────────────────────────────────────────────────────────────────

    address public treasury;
    uint256 public currentCohortId;
    uint256 private _nextTokenId;
    uint256 private _nextAppId;

    /// @notice Tier prices in WMON
    mapping(Tier => uint256) public tierPrice;

    /// @notice cohortId => Cohort data
    mapping(uint256 => Cohort) public cohorts;

    /// @notice cohortId => member address => Member data
    mapping(uint256 => mapping(address => Member)) public members;

    /// @notice tokenId => member address
    mapping(uint256 => address) public tokenMember;

    /// @notice tokenId => cohortId
    mapping(uint256 => uint256) public tokenCohort;

    /// @notice Application ID => Application data
    mapping(uint256 => Application) public applications;

    /// @notice applicant address => latest application ID (0 = never applied)
    mapping(address => uint256) public applicantApp;

    /// @notice address => approved to join (whitelist)
    mapping(uint256 => mapping(address => bool)) public whitelisted;

    // ─── Events ──────────────────────────────────────────────────────────────────

    event CohortStarted(uint256 indexed cohortId, uint256 startTime, uint256 duration);
    event CohortEnded(uint256 indexed cohortId, uint256 endTime);
    event CohortFinalized(uint256 indexed cohortId);
    event TierPricesUpdated(uint256 explorer, uint256 builder, uint256 founder);
    event ApplicationSubmitted(uint256 indexed appId, address indexed applicant, Tier tier);
    event ApplicationApproved(uint256 indexed appId, address indexed applicant);
    event ApplicationRejected(uint256 indexed appId, address indexed applicant);
    event MemberJoined(uint256 indexed cohortId, address indexed member, Tier tier);
    event PaymentReceived(
        uint256 indexed cohortId,
        address indexed member,
        uint256 amount,
        uint256 fee,
        uint256 monthsPaid
    );
    event MembershipCardMinted(uint256 indexed tokenId, address indexed member, uint256 indexed cohortId);
    event ToursAirdropped(address indexed recipient, uint256 amount);
    event FounderSelected(uint256 indexed cohortId, address indexed founder, uint256 amount);
    event TierUpgraded(address indexed member, Tier oldTier, Tier newTier);
    event MemberBanned(uint256 indexed cohortId, address indexed member);
    event TreasuryUpdated(address oldTreasury, address newTreasury);
    event TokenRescued(address indexed token, address indexed to, uint256 amount);

    // ─── Errors ──────────────────────────────────────────────────────────────────

    error InvalidTier();
    error NotWhitelisted();
    error AlreadyApplied();
    error ApplicationNotPending();
    error SoulboundTransferBlocked();

    // ─── Constructor ─────────────────────────────────────────────────────────────

    constructor(
        address _wmon,
        address _tours,
        address _treasury
    ) ERC721("EmpowerTours Membership", "ETMEM") Ownable(msg.sender) {
        require(_wmon != address(0), "zero wmon");
        require(_tours != address(0), "zero tours");
        require(_treasury != address(0), "zero treasury");
        wmon = IERC20(_wmon);
        tours = IERC20(_tours);
        treasury = _treasury;
        _nextTokenId = 1;
        _nextAppId = 1;
    }

    // ─── Public: Application ─────────────────────────────────────────────────────

    /// @notice Submit a membership application. Screened and approved by admin.
    /// @param requestedTier The tier the applicant wants
    /// @param name Applicant's name
    /// @param reason Why they want to join
    function applyForMembership(
        Tier requestedTier,
        string calldata name,
        string calldata reason
    ) external {
        if (requestedTier == Tier.None) revert InvalidTier();
        if (applicantApp[msg.sender] != 0) {
            // Allow re-application only if previously rejected
            require(
                applications[applicantApp[msg.sender]].status == AppStatus.Rejected,
                "already applied"
            );
        }

        uint256 appId = _nextAppId++;
        applications[appId] = Application({
            applicant: msg.sender,
            requestedTier: requestedTier,
            name: name,
            reason: reason,
            status: AppStatus.Pending,
            submittedAt: block.timestamp,
            reviewedAt: 0
        });
        applicantApp[msg.sender] = appId;

        emit ApplicationSubmitted(appId, msg.sender, requestedTier);
    }

    // ─── Admin: Application Review ───────────────────────────────────────────────

    /// @notice Approve an application and whitelist the applicant for the current cohort.
    function approveApplication(uint256 appId) external onlyOwner {
        Application storage app = applications[appId];
        if (app.status != AppStatus.Pending) revert ApplicationNotPending();

        app.status = AppStatus.Approved;
        app.reviewedAt = block.timestamp;

        // Auto-whitelist for current cohort
        if (currentCohortId > 0 && cohorts[currentCohortId].active) {
            _whitelistMember(currentCohortId, app.applicant, app.requestedTier);
        }

        emit ApplicationApproved(appId, app.applicant);
    }

    /// @notice Reject an application.
    function rejectApplication(uint256 appId) external onlyOwner {
        Application storage app = applications[appId];
        if (app.status != AppStatus.Pending) revert ApplicationNotPending();

        app.status = AppStatus.Rejected;
        app.reviewedAt = block.timestamp;

        emit ApplicationRejected(appId, app.applicant);
    }

    /// @notice Batch approve multiple applications.
    function batchApproveApplications(uint256[] calldata appIds) external onlyOwner {
        require(appIds.length <= MAX_BATCH_SIZE, "batch too large");
        for (uint256 i = 0; i < appIds.length; i++) {
            Application storage app = applications[appIds[i]];
            if (app.status != AppStatus.Pending) revert ApplicationNotPending();

            app.status = AppStatus.Approved;
            app.reviewedAt = block.timestamp;

            if (currentCohortId > 0 && cohorts[currentCohortId].active) {
                _whitelistMember(currentCohortId, app.applicant, app.requestedTier);
            }

            emit ApplicationApproved(appIds[i], app.applicant);
        }
    }

    // ─── Admin: Cohort Lifecycle ─────────────────────────────────────────────────

    /// @notice Start a new cohort. Deactivates any active cohort.
    function startCohort(uint256 duration) external onlyOwner {
        require(duration >= 30 days && duration <= 730 days, "invalid duration");

        if (currentCohortId > 0 && cohorts[currentCohortId].active) {
            cohorts[currentCohortId].active = false;
            cohorts[currentCohortId].endTime = block.timestamp;
            emit CohortEnded(currentCohortId, block.timestamp);
        }

        currentCohortId++;
        cohorts[currentCohortId] = Cohort({
            startTime: block.timestamp,
            endTime: 0,
            duration: duration,
            poolBalance: 0,
            totalCollected: 0,
            memberCount: 0,
            active: true,
            finalized: false
        });

        emit CohortStarted(currentCohortId, block.timestamp, duration);
    }

    /// @notice End the current cohort.
    function endCohort() external onlyOwner {
        Cohort storage c = cohorts[currentCohortId];
        require(c.active, "no active cohort");
        c.active = false;
        c.endTime = block.timestamp;
        emit CohortEnded(currentCohortId, block.timestamp);
    }

    // ─── Admin: Configuration ────────────────────────────────────────────────────

    /// @notice Set WMON prices per tier.
    function setTierPrices(
        uint256 explorer,
        uint256 builder,
        uint256 founder
    ) external onlyOwner {
        require(explorer > 0 && builder > 0 && founder > 0, "zero price");
        require(explorer <= MAX_TIER_PRICE, "explorer exceeds max");
        require(builder <= MAX_TIER_PRICE, "builder exceeds max");
        require(founder <= MAX_TIER_PRICE, "founder exceeds max");
        require(explorer <= builder && builder <= founder, "invalid tier ordering");

        tierPrice[Tier.Explorer] = explorer;
        tierPrice[Tier.Builder] = builder;
        tierPrice[Tier.Founder] = founder;

        emit TierPricesUpdated(explorer, builder, founder);
    }

    /// @notice Update treasury address.
    function setTreasury(address newTreasury) external onlyOwner {
        require(newTreasury != address(0), "zero treasury");
        emit TreasuryUpdated(treasury, newTreasury);
        treasury = newTreasury;
    }

    // ─── Admin: Whitelist ────────────────────────────────────────────────────────

    /// @notice Batch whitelist approved candidates for the current cohort.
    function whitelistCandidates(
        address[] calldata candidates,
        Tier[] calldata tiers
    ) external onlyOwner {
        require(candidates.length == tiers.length, "length mismatch");
        require(candidates.length <= MAX_BATCH_SIZE, "batch too large");

        for (uint256 i = 0; i < candidates.length; i++) {
            _whitelistMember(currentCohortId, candidates[i], tiers[i]);
        }
    }

    /// @notice Upgrade a member's tier.
    function upgradeTier(address member, Tier newTier) external onlyOwner {
        Member storage m = members[currentCohortId][member];
        require(m.tier != Tier.None, "not a member");
        require(newTier > m.tier, "can only upgrade");

        Tier oldTier = m.tier;
        m.tier = newTier;

        emit TierUpgraded(member, oldTier, newTier);
    }

    /// @notice Ban a member.
    function banMember(uint256 cohortId, address member) external onlyOwner {
        Member storage m = members[cohortId][member];
        require(m.tier != Tier.None, "not a member");
        require(!m.banned, "already banned");

        m.banned = true;
        emit MemberBanned(cohortId, member);
    }

    // ─── Member: Payment ─────────────────────────────────────────────────────────

    /// @notice Pay monthly tier fee. Must be whitelisted (via approved application).
    ///         Auto-registers on first payment, mints soulbound membership NFT.
    function payMonthly() external nonReentrant {
        Cohort storage c = cohorts[currentCohortId];
        require(c.active, "no active cohort");

        // Must be whitelisted (approved application)
        require(whitelisted[currentCohortId][msg.sender], "not whitelisted");

        Member storage m = members[currentCohortId][msg.sender];
        require(!m.banned, "member banned");

        // First payment — join cohort
        if (m.monthsPaid == 0) {
            if (m.tier == Tier.None) {
                revert NotWhitelisted();
            }
            c.memberCount++;
            emit MemberJoined(currentCohortId, msg.sender, m.tier);
        }

        require(m.monthsPaid < MAX_PAYMENTS, "max payments reached");
        require(
            m.lastPaymentTime == 0 ||
            block.timestamp >= m.lastPaymentTime + MIN_PAYMENT_INTERVAL,
            "too soon"
        );

        uint256 price = tierPrice[m.tier];
        require(price > 0, "tier price not set");

        uint256 fee = (price * TREASURY_FEE_BPS) / BPS_DENOMINATOR;
        uint256 toPool = price - fee;

        // Effects before interactions
        m.monthsPaid++;
        m.lastPaymentTime = block.timestamp;
        m.totalPaid += price;
        c.poolBalance += toPool;
        c.totalCollected += price;

        // Mint membership card on first payment
        uint256 mintedTokenId = 0;
        if (m.tokenId == 0) {
            mintedTokenId = _nextTokenId++;
            m.tokenId = mintedTokenId;
            tokenMember[mintedTokenId] = msg.sender;
            tokenCohort[mintedTokenId] = currentCohortId;
        }

        // External calls after state changes
        wmon.safeTransferFrom(msg.sender, treasury, fee);
        wmon.safeTransferFrom(msg.sender, address(this), toPool);

        if (mintedTokenId > 0) {
            _mint(msg.sender, mintedTokenId);
            emit MembershipCardMinted(mintedTokenId, msg.sender, currentCohortId);
        }

        emit PaymentReceived(currentCohortId, msg.sender, price, fee, m.monthsPaid);
    }

    // ─── Admin: Airdrops ─────────────────────────────────────────────────────────

    /// @notice Load TOURS tokens for airdrops.
    function fundTours(uint256 amount) external onlyOwner {
        require(amount > 0, "zero amount");
        tours.safeTransferFrom(msg.sender, address(this), amount);
    }

    /// @notice Batch airdrop TOURS to members.
    function airdropTours(
        address[] calldata recipients,
        uint256[] calldata amounts
    ) external onlyOwner {
        require(recipients.length == amounts.length, "length mismatch");
        require(recipients.length <= MAX_BATCH_SIZE, "batch too large");

        for (uint256 i = 0; i < recipients.length; i++) {
            require(recipients[i] != address(0), "zero address");
            require(amounts[i] > 0, "zero amount");
            tours.safeTransfer(recipients[i], amounts[i]);
            emit ToursAirdropped(recipients[i], amounts[i]);
        }
    }

    // ─── Admin: Founder Distribution ─────────────────────────────────────────────

    /// @notice Distribute pool WMON to graduating founders.
    function selectFounders(
        uint256 cohortId,
        address[] calldata founders,
        uint256[] calldata amounts
    ) external onlyOwner nonReentrant {
        require(founders.length == amounts.length, "length mismatch");
        require(founders.length <= MAX_BATCH_SIZE, "batch too large");

        Cohort storage c = cohorts[cohortId];
        require(c.startTime > 0, "cohort does not exist");
        require(!c.active, "cohort still active");
        require(!c.finalized, "cohort finalized");

        for (uint256 i = 0; i < founders.length; i++) {
            require(founders[i] != address(0), "zero address");
            Member storage m = members[cohortId][founders[i]];
            require(m.tier != Tier.None, "not a member");
            require(!m.banned, "member banned");
            require(m.monthsPaid > 0, "never paid");
            require(!m.isFounder, "already selected");

            uint256 amount = amounts[i];
            require(amount <= c.poolBalance, "exceeds pool balance");

            c.poolBalance -= amount;
            m.isFounder = true;
            m.founderAmount = amount;

            wmon.safeTransfer(founders[i], amount);
            emit FounderSelected(cohortId, founders[i], amount);
        }
    }

    /// @notice Finalize a cohort.
    function finalizeCohort(uint256 cohortId) external onlyOwner {
        Cohort storage c = cohorts[cohortId];
        require(c.startTime > 0, "cohort does not exist");
        require(!c.active, "cohort still active");
        require(!c.finalized, "already finalized");

        c.finalized = true;
        emit CohortFinalized(cohortId);
    }

    /// @notice Withdraw leftover pool after finalization.
    function withdrawRemainingPool(uint256 cohortId, address to, uint256 amount) external onlyOwner {
        require(to != address(0), "zero address");
        Cohort storage c = cohorts[cohortId];
        require(c.finalized, "not finalized");
        require(amount <= c.poolBalance, "exceeds pool balance");

        c.poolBalance -= amount;
        wmon.safeTransfer(to, amount);
    }

    // ─── Admin: Rescue ───────────────────────────────────────────────────────────

    /// @notice Rescue tokens accidentally sent to the contract.
    function rescueTokens(address token, address to, uint256 amount) external onlyOwner {
        require(to != address(0), "zero address");
        require(amount > 0, "zero amount");

        if (token == address(wmon)) {
            uint256 totalPooled = 0;
            for (uint256 i = 1; i <= currentCohortId; i++) {
                totalPooled += cohorts[i].poolBalance;
            }
            require(amount <= wmon.balanceOf(address(this)) - totalPooled, "would undercut pool");
        }

        IERC20(token).safeTransfer(to, amount);
        emit TokenRescued(token, to, amount);
    }

    // ─── ERC721: Soulbound ───────────────────────────────────────────────────────

    /// @notice Block all transfers. Mint and burn only.
    function _update(address to, uint256 tokenId, address auth) internal override returns (address) {
        address from = _ownerOf(tokenId);
        if (from != address(0) && to != address(0)) {
            revert SoulboundTransferBlocked();
        }
        return super._update(to, tokenId, auth);
    }

    // ─── Views ───────────────────────────────────────────────────────────────────

    /// @notice Check if address holds a membership NFT (any cohort).
    function isMember(address wallet) external view returns (bool) {
        return balanceOf(wallet) > 0;
    }

    /// @notice Get member info for a cohort.
    function getMember(uint256 cohortId, address member) external view returns (Member memory) {
        return members[cohortId][member];
    }

    /// @notice Get cohort info.
    function getCohort(uint256 cohortId) external view returns (Cohort memory) {
        return cohorts[cohortId];
    }

    /// @notice Get application info.
    function getApplication(uint256 appId) external view returns (Application memory) {
        return applications[appId];
    }

    /// @notice Total NFTs minted.
    function totalSupply() external view returns (uint256) {
        return _nextTokenId - 1;
    }

    /// @notice Total applications submitted.
    function totalApplications() external view returns (uint256) {
        return _nextAppId - 1;
    }

    // ─── Internal ────────────────────────────────────────────────────────────────

    function _whitelistMember(uint256 cohortId, address candidate, Tier tier) internal {
        require(candidate != address(0), "zero address");
        if (tier == Tier.None) revert InvalidTier();
        require(
            members[cohortId][candidate].tier == Tier.None,
            "already whitelisted"
        );

        Cohort storage c = cohorts[cohortId];
        require(c.active, "no active cohort");

        whitelisted[cohortId][candidate] = true;
        members[cohortId][candidate] = Member({
            tier: tier,
            cohortId: cohortId,
            monthsPaid: 0,
            lastPaymentTime: 0,
            totalPaid: 0,
            isFounder: false,
            founderAmount: 0,
            tokenId: 0,
            banned: false
        });
    }

    /// @notice Reject native MON sent directly.
    receive() external payable {
        revert("use WMON");
    }
}
