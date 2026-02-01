import React from 'react';
import { motion } from 'framer-motion';
import { Coins, Lock, BarChart3, Zap, Shield, ExternalLink, Users, TrendingDown } from 'lucide-react';
import TokenAllocationChart from './TokenAllocationChart';

const TOKENOMICS = {
  token: { name: 'TOURS', symbol: 'TOURS', chain: 'Monad', chainId: 143 },
  contract: '0xf61F2b014e38FfEf66a3A0a8104D36365404f74f',
  maxSupply: 100_000_000_000,
  allocations: [
    { name: 'Community Rewards', pct: 40, amount: '40B', vesting: 'Halving over ~10 years', color: '#836EF9' },
    { name: 'Treasury / DAO', pct: 20, amount: '20B', vesting: 'DAO-governed', color: '#00f3ff' },
    { name: 'Team & Founders', pct: 15, amount: '15B', vesting: '4yr vest, 1yr cliff', color: '#10b981' },
    { name: 'Investors', pct: 15, amount: '15B', vesting: '2yr vest, 6mo cliff', color: '#f59e0b' },
    { name: 'Liquidity Pool', pct: 10, amount: '10B', vesting: 'At TGE', color: '#3b82f6' },
  ],
  contracts: {
    toursToken: '0xf61F2b014e38FfEf66a3A0a8104D36365404f74f',
    votingTours: '0xe5377b1f90B9A70dD7B0f6Ea34F9c3D287B3C44c',
    rewardManager: '0x7fff35BB27307806B92Fb1D1FBe52D168093eF87',
    nftV3: '0xB9B3acf33439360B55d12429301E946f34f3B73F',
    governor: '0x4d05fb8c2D090769A084aa0138cCF7A549452Fa3',
    timelock: '0x4F7F9111215F2270A92Bd64e4c1E9D7De516bd79',
    daoFactory: '0x627a2c457e5Eb3E9C4B6632Ac69f8c39228D7968',
    platformSafe: '0xf3b9D123E7Ac8C36FC9b5AB32135c665956725bA',
  },
  revenue: {
    musicNFT: { artist: 70, platform: 30 },
    subscriptions: { artists: 70, daoReserve: 20, treasury: 10 },
    radio: { artist: 70, platformSafe: 15, platformWallet: 15 },
    passports: '150 WMON per mint',
    itineraries: { creator: 70, platform: 30 },
    climbing: { creator: 70, platform: 30 },
  },
};

const halvingSchedule = [
  { epoch: 1, period: 'Year 1-2', rewardsPerEpoch: '10B', cumulative: '10B', pctOfPool: '25%' },
  { epoch: 2, period: 'Year 2-3', rewardsPerEpoch: '5B', cumulative: '15B', pctOfPool: '37.5%' },
  { epoch: 3, period: 'Year 3-4', rewardsPerEpoch: '2.5B', cumulative: '17.5B', pctOfPool: '43.75%' },
  { epoch: 4, period: 'Year 4-5', rewardsPerEpoch: '1.25B', cumulative: '18.75B', pctOfPool: '46.9%' },
  { epoch: 5, period: 'Year 5-6', rewardsPerEpoch: '625M', cumulative: '19.375B', pctOfPool: '48.4%' },
  { epoch: 6, period: 'Year 6-7', rewardsPerEpoch: '312.5M', cumulative: '19.69B', pctOfPool: '49.2%' },
  { epoch: 7, period: 'Year 7-8', rewardsPerEpoch: '156.25M', cumulative: '19.84B', pctOfPool: '49.6%' },
  { epoch: 8, period: 'Year 8-9', rewardsPerEpoch: '78.1M', cumulative: '19.92B', pctOfPool: '49.8%' },
  { epoch: 9, period: 'Year 9-10', rewardsPerEpoch: '39M', cumulative: '19.96B', pctOfPool: '49.9%' },
  { epoch: 10, period: 'Year 10+', rewardsPerEpoch: '19.5M', cumulative: '~20B', pctOfPool: '~50%' },
];

const contractEntries = [
  { label: 'TOURS Token', address: TOKENOMICS.contracts.toursToken },
  { label: 'Voting TOURS (vTOURS)', address: TOKENOMICS.contracts.votingTours },
  { label: 'Reward Manager', address: TOKENOMICS.contracts.rewardManager },
  { label: 'NFT V3 Contract', address: TOKENOMICS.contracts.nftV3 },
  { label: 'Governor', address: TOKENOMICS.contracts.governor },
  { label: 'Timelock Controller', address: TOKENOMICS.contracts.timelock },
  { label: 'DAO Factory', address: TOKENOMICS.contracts.daoFactory },
  { label: 'Platform Safe', address: TOKENOMICS.contracts.platformSafe },
];

const revenueStreams = [
  { name: 'Music NFTs', splits: ['70% Artist', '30% Platform'], icon: '🎵' },
  { name: 'Subscriptions', splits: ['70% Artists', '20% DAO Reserve', '10% Treasury'], icon: '🎧' },
  { name: 'Radio Plays', splits: ['70% Artist', '15% Platform Safe', '15% Platform'], icon: '📻' },
  { name: 'Passport Mints', splits: ['150 WMON per mint'], icon: '🛂' },
  { name: 'Itineraries', splits: ['70% Creator', '30% Platform'], icon: '🗺️' },
  { name: 'Climbing Guides', splits: ['70% Creator', '30% Platform'], icon: '🧗' },
];

const tokenUtilities = [
  { title: 'Governance', description: 'Wrap TOURS into vTOURS to vote on DAO proposals and treasury allocations.', icon: Shield },
  { title: 'Staking Rewards', description: 'Stake vTOURS to earn platform revenue share and bonus rewards.', icon: Coins },
  { title: 'Platform Payments', description: 'Use TOURS for passport mints, NFT purchases, subscriptions, and itinerary access.', icon: Zap },
  { title: 'Burn Mechanics', description: 'A portion of platform fees are used to buy back and burn TOURS, reducing supply over time.', icon: TrendingDown },
];

const SectionHeading: React.FC<{ title: string; subtitle?: string; icon?: React.ReactNode }> = ({ title, subtitle, icon }) => (
  <div className="mb-8 md:mb-12">
    <div className="flex items-center gap-3 mb-3">
      {icon}
      <h2 className="text-2xl md:text-4xl font-heading font-bold text-white uppercase tracking-wide">{title}</h2>
    </div>
    {subtitle && <p className="text-gray-400 text-sm md:text-base max-w-2xl">{subtitle}</p>}
  </div>
);

const TokenomicsPage: React.FC = () => {
  return (
    <section id="tokenomics" className="relative z-10 py-16 md:py-32 bg-black/30 backdrop-blur-sm border-y border-white/5">
      <div className="max-w-7xl mx-auto px-4 md:px-6">

        {/* Hero */}
        <motion.div
          initial={{ opacity: 0, y: 30 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8 }}
          viewport={{ once: true }}
          className="text-center mb-16 md:mb-24"
        >
          <div className="inline-flex items-center gap-2 text-[10px] md:text-xs font-mono text-[#00f3ff] tracking-[0.3em] uppercase mb-6 bg-[#00f3ff]/5 border border-[#00f3ff]/20 px-5 py-2 rounded-full">
            <Coins className="w-3 h-3" /> Token Economics
          </div>
          <h1 className="text-4xl md:text-7xl font-heading font-bold uppercase leading-tight mb-4">
            <span className="text-transparent bg-clip-text bg-gradient-to-r from-[#836EF9] via-[#00f3ff] to-[#836EF9] bg-[length:200%_auto] animate-[gradient_6s_linear_infinite]">
              TOURS
            </span>{' '}
            <span className="text-transparent bg-clip-text bg-gradient-to-b from-white to-gray-600">
              Tokenomics
            </span>
          </h1>
          <p className="text-gray-400 text-base md:text-lg max-w-2xl mx-auto leading-relaxed">
            The economic engine powering EmpowerTours on Monad. 100 billion tokens driving governance, rewards, and a creator-first economy.
          </p>
        </motion.div>

        {/* Supply Overview */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-16 md:mb-24"
        >
          {[
            { label: 'Total Supply', value: '100B', sub: 'TOURS' },
            { label: 'Chain', value: 'Monad', sub: 'Chain ID 143' },
            { label: 'Token Standard', value: 'ERC-20', sub: 'With Permit' },
            { label: 'Governance', value: 'vTOURS', sub: 'Wrap to vote' },
          ].map((item, i) => (
            <div key={i} className="p-4 md:p-6 bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none text-center">
              <div className="text-xs text-gray-500 uppercase tracking-widest mb-2 font-mono">{item.label}</div>
              <div className="text-2xl md:text-3xl font-heading font-bold text-white">{item.value}</div>
              <div className="text-xs text-gray-500 mt-1">{item.sub}</div>
            </div>
          ))}
        </motion.div>

        {/* Allocation Chart */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="mb-16 md:mb-24"
        >
          <SectionHeading
            title="Token Allocation"
            subtitle="Distribution designed for long-term sustainability and community ownership."
            icon={<BarChart3 className="w-6 h-6 text-[#836EF9]" />}
          />
          <div className="bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none p-6 md:p-10">
            <TokenAllocationChart allocations={TOKENOMICS.allocations} />
          </div>
        </motion.div>

        {/* Vesting Timeline */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="mb-16 md:mb-24"
        >
          <SectionHeading
            title="Vesting Schedule"
            subtitle="Structured unlocks to align long-term incentives."
            icon={<Lock className="w-6 h-6 text-[#00f3ff]" />}
          />
          <div className="relative">
            {/* Timeline line */}
            <div className="hidden md:block absolute left-8 top-0 bottom-0 w-px bg-gradient-to-b from-[#836EF9] via-[#00f3ff] to-[#836EF9]/20" />
            <div className="space-y-4">
              {[
                { label: 'TGE', time: 'Day 0', items: ['Liquidity Pool (10B)', 'Initial DAO Treasury allocation'] },
                { label: '6 months', time: 'Month 6', items: ['Investor cliff ends, linear unlock begins'] },
                { label: '1 year', time: 'Month 12', items: ['Team cliff ends, linear unlock begins', 'First halving epoch complete'] },
                { label: '2 years', time: 'Month 24', items: ['Investor vesting complete (15B fully unlocked)', 'Second halving epoch'] },
                { label: '4 years', time: 'Month 48', items: ['Team vesting complete (15B fully unlocked)', 'Fourth halving epoch'] },
                { label: '10 years', time: 'Year 10', items: ['~50% of community rewards distributed', 'Long-tail emissions continue'] },
              ].map((milestone, i) => (
                <div key={i} className="flex gap-4 md:gap-8 items-start">
                  <div className="hidden md:flex flex-col items-center">
                    <div className="w-4 h-4 rounded-full bg-[#836EF9] border-2 border-[#0a0a12] z-10 flex-shrink-0" />
                  </div>
                  <div className="flex-1 bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none p-4 md:p-6">
                    <div className="flex items-baseline gap-3 mb-2">
                      <span className="text-sm font-heading font-bold text-[#836EF9] uppercase">{milestone.label}</span>
                      <span className="text-xs text-gray-500 font-mono">{milestone.time}</span>
                    </div>
                    <ul className="space-y-1">
                      {milestone.items.map((item, j) => (
                        <li key={j} className="text-sm text-gray-300 flex items-start gap-2">
                          <span className="w-1 h-1 bg-[#00f3ff] rounded-full mt-2 flex-shrink-0" />
                          {item}
                        </li>
                      ))}
                    </ul>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </motion.div>

        {/* Token Utility */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="mb-16 md:mb-24"
        >
          <SectionHeading
            title="Token Utility"
            subtitle="TOURS powers every interaction on the platform."
            icon={<Zap className="w-6 h-6 text-[#f59e0b]" />}
          />
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
            {tokenUtilities.map((util, i) => (
              <div key={i} className="p-6 bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none group hover:border-[#836EF9]/30 transition-colors">
                <div className="w-10 h-10 rounded-full bg-[#836EF9]/10 flex items-center justify-center mb-4 group-hover:bg-[#836EF9]/20 transition-colors border border-[#836EF9]/20">
                  <util.icon className="w-5 h-5 text-[#836EF9]" />
                </div>
                <h3 className="text-lg font-bold text-white mb-2 font-heading">{util.title}</h3>
                <p className="text-sm text-gray-400 leading-relaxed">{util.description}</p>
              </div>
            ))}
          </div>
        </motion.div>

        {/* Revenue Model */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="mb-16 md:mb-24"
        >
          <SectionHeading
            title="Revenue Model"
            subtitle="Creator-first economics with transparent platform fees."
            icon={<BarChart3 className="w-6 h-6 text-[#10b981]" />}
          />
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {revenueStreams.map((stream, i) => (
              <div key={i} className="p-5 bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none">
                <div className="flex items-center gap-3 mb-3">
                  <span className="text-2xl">{stream.icon}</span>
                  <h3 className="text-base font-bold text-white">{stream.name}</h3>
                </div>
                <div className="space-y-1.5">
                  {stream.splits.map((split, j) => (
                    <div key={j} className="text-sm text-gray-400 flex items-center gap-2">
                      <span className="w-1.5 h-1.5 bg-[#10b981] rounded-full flex-shrink-0" />
                      {split}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </motion.div>

        {/* Halving Schedule */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="mb-16 md:mb-24"
        >
          <SectionHeading
            title="Halving Schedule"
            subtitle="Community reward emissions halve each epoch, following Bitcoin-inspired deflation."
            icon={<TrendingDown className="w-6 h-6 text-[#836EF9]" />}
          />
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/10">
                  <th className="text-left py-3 px-4 text-xs text-gray-500 uppercase tracking-wider font-mono">Epoch</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 uppercase tracking-wider font-mono">Period</th>
                  <th className="text-right py-3 px-4 text-xs text-gray-500 uppercase tracking-wider font-mono">Rewards</th>
                  <th className="text-right py-3 px-4 text-xs text-gray-500 uppercase tracking-wider font-mono">Cumulative</th>
                  <th className="text-right py-3 px-4 text-xs text-gray-500 uppercase tracking-wider font-mono hidden md:table-cell">% of Pool</th>
                </tr>
              </thead>
              <tbody>
                {halvingSchedule.map((row, i) => (
                  <tr key={i} className="border-b border-white/5 hover:bg-white/[0.02] transition-colors">
                    <td className="py-3 px-4 text-[#836EF9] font-bold font-mono">{row.epoch}</td>
                    <td className="py-3 px-4 text-gray-300">{row.period}</td>
                    <td className="py-3 px-4 text-right text-white font-mono">{row.rewardsPerEpoch}</td>
                    <td className="py-3 px-4 text-right text-gray-400 font-mono">{row.cumulative}</td>
                    <td className="py-3 px-4 text-right text-gray-500 font-mono hidden md:table-cell">{row.pctOfPool}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </motion.div>

        {/* Contract Addresses */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
          className="mb-16 md:mb-24"
        >
          <SectionHeading
            title="Contract Addresses"
            subtitle="All contracts deployed on Monad Mainnet (Chain ID 143). Verified on MonadScan."
            icon={<Shield className="w-6 h-6 text-[#00f3ff]" />}
          />
          <div className="space-y-2">
            {contractEntries.map((entry, i) => (
              <a
                key={i}
                href={`https://monadscan.com/address/${entry.address}`}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center justify-between p-4 bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none hover:border-[#00f3ff]/30 transition-colors group"
                data-hover="true"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <div className="w-2 h-2 rounded-full bg-[#10b981] flex-shrink-0" />
                  <span className="text-sm font-semibold text-white">{entry.label}</span>
                </div>
                <div className="flex items-center gap-2">
                  <code className="text-xs text-gray-500 font-mono hidden md:inline">
                    {entry.address}
                  </code>
                  <code className="text-xs text-gray-500 font-mono md:hidden">
                    {entry.address.slice(0, 6)}...{entry.address.slice(-4)}
                  </code>
                  <ExternalLink className="w-3.5 h-3.5 text-gray-600 group-hover:text-[#00f3ff] transition-colors flex-shrink-0" />
                </div>
              </a>
            ))}
          </div>
        </motion.div>

        {/* Live Stats Placeholder */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6 }}
          viewport={{ once: true }}
        >
          <SectionHeading
            title="Live Stats"
            subtitle="On-chain metrics updated in real-time."
            icon={<Users className="w-6 h-6 text-[#836EF9]" />}
          />
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            {[
              { label: 'Holders', value: '--', sub: 'Coming soon' },
              { label: 'Circulating Supply', value: '--', sub: 'Coming soon' },
              { label: 'Reward Pool Balance', value: '--', sub: 'Coming soon' },
              { label: 'Total Burned', value: '--', sub: 'Coming soon' },
            ].map((stat, i) => (
              <div key={i} className="p-4 md:p-6 bg-[#0a0a12] border border-white/5 rounded-lg md:rounded-none text-center">
                <div className="text-xs text-gray-500 uppercase tracking-widest mb-2 font-mono">{stat.label}</div>
                <div className="text-2xl font-heading font-bold text-gray-600">{stat.value}</div>
                <div className="text-xs text-gray-600 mt-1">{stat.sub}</div>
              </div>
            ))}
          </div>
        </motion.div>
      </div>
    </section>
  );
};

export default TokenomicsPage;
