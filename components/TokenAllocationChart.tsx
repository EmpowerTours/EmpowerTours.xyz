import React, { useState } from 'react';
import { motion } from 'framer-motion';

interface Allocation {
  name: string;
  pct: number;
  amount: string;
  vesting: string;
  color: string;
}

interface TokenAllocationChartProps {
  allocations: Allocation[];
}

const TokenAllocationChart: React.FC<TokenAllocationChartProps> = ({ allocations }) => {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);

  const size = 300;
  const center = size / 2;
  const radius = 110;
  const innerRadius = 70;
  const strokeWidth = radius - innerRadius;
  const midRadius = (radius + innerRadius) / 2;

  // Build arc segments
  const segments: Array<{
    d: string;
    allocation: Allocation;
    index: number;
    midAngle: number;
  }> = [];

  let currentAngle = -90; // start from top

  allocations.forEach((alloc, index) => {
    const angle = (alloc.pct / 100) * 360;
    const startAngle = currentAngle;
    const endAngle = currentAngle + angle;
    const midAngle = startAngle + angle / 2;

    const startRad = (startAngle * Math.PI) / 180;
    const endRad = (endAngle * Math.PI) / 180;

    const x1Outer = center + radius * Math.cos(startRad);
    const y1Outer = center + radius * Math.sin(startRad);
    const x2Outer = center + radius * Math.cos(endRad);
    const y2Outer = center + radius * Math.sin(endRad);

    const x1Inner = center + innerRadius * Math.cos(endRad);
    const y1Inner = center + innerRadius * Math.sin(endRad);
    const x2Inner = center + innerRadius * Math.cos(startRad);
    const y2Inner = center + innerRadius * Math.sin(startRad);

    const largeArc = angle > 180 ? 1 : 0;

    const d = [
      `M ${x1Outer} ${y1Outer}`,
      `A ${radius} ${radius} 0 ${largeArc} 1 ${x2Outer} ${y2Outer}`,
      `L ${x1Inner} ${y1Inner}`,
      `A ${innerRadius} ${innerRadius} 0 ${largeArc} 0 ${x2Inner} ${y2Inner}`,
      'Z',
    ].join(' ');

    segments.push({ d, allocation: alloc, index, midAngle });
    currentAngle = endAngle;
  });

  return (
    <div className="flex flex-col md:flex-row items-center gap-8 md:gap-12">
      {/* Chart */}
      <div className="relative flex-shrink-0">
        <svg
          width={size}
          height={size}
          viewBox={`0 0 ${size} ${size}`}
          className="drop-shadow-[0_0_30px_rgba(131,110,249,0.2)]"
        >
          {segments.map(({ d, allocation, index, midAngle }) => {
            const isHovered = hoveredIndex === index;
            const midRad = (midAngle * Math.PI) / 180;
            const offsetX = isHovered ? Math.cos(midRad) * 6 : 0;
            const offsetY = isHovered ? Math.sin(midRad) * 6 : 0;

            return (
              <g key={index}>
                <motion.path
                  d={d}
                  fill={allocation.color}
                  opacity={hoveredIndex !== null && !isHovered ? 0.4 : 1}
                  animate={{
                    x: offsetX,
                    y: offsetY,
                    opacity: hoveredIndex !== null && !isHovered ? 0.4 : 1,
                  }}
                  transition={{ type: 'spring', stiffness: 300, damping: 20 }}
                  onMouseEnter={() => setHoveredIndex(index)}
                  onMouseLeave={() => setHoveredIndex(null)}
                  className="cursor-pointer"
                  style={{ filter: isHovered ? `drop-shadow(0 0 8px ${allocation.color})` : 'none' }}
                />
                {/* Percentage label on arc */}
                {allocation.pct >= 10 && (
                  <text
                    x={center + midRadius * Math.cos(midRad)}
                    y={center + midRadius * Math.sin(midRad)}
                    textAnchor="middle"
                    dominantBaseline="central"
                    fill="#fff"
                    fontSize="12"
                    fontWeight="bold"
                    className="pointer-events-none select-none"
                    style={{ textShadow: '0 1px 3px rgba(0,0,0,0.8)' }}
                  >
                    {allocation.pct}%
                  </text>
                )}
              </g>
            );
          })}

          {/* Center text */}
          <text
            x={center}
            y={center - 10}
            textAnchor="middle"
            fill="#fff"
            fontSize="16"
            fontWeight="bold"
            className="select-none"
          >
            100B
          </text>
          <text
            x={center}
            y={center + 10}
            textAnchor="middle"
            fill="#9ca3af"
            fontSize="11"
            className="select-none"
          >
            TOURS
          </text>
        </svg>

        {/* Hover tooltip */}
        {hoveredIndex !== null && (
          <motion.div
            initial={{ opacity: 0, y: 5 }}
            animate={{ opacity: 1, y: 0 }}
            className="absolute -bottom-2 left-1/2 -translate-x-1/2 bg-black/90 border border-white/10 rounded-lg px-4 py-2 text-center whitespace-nowrap backdrop-blur-md z-10"
          >
            <div className="text-sm font-bold text-white">{allocations[hoveredIndex].name}</div>
            <div className="text-xs text-gray-400">
              {allocations[hoveredIndex].amount} ({allocations[hoveredIndex].pct}%)
            </div>
            <div className="text-xs text-gray-500 mt-0.5">{allocations[hoveredIndex].vesting}</div>
          </motion.div>
        )}
      </div>

      {/* Legend */}
      <div className="flex flex-col gap-3 w-full md:w-auto">
        {allocations.map((alloc, index) => (
          <div
            key={index}
            className={`flex items-center gap-3 p-2 rounded-lg transition-all cursor-pointer ${
              hoveredIndex === index ? 'bg-white/5' : ''
            }`}
            onMouseEnter={() => setHoveredIndex(index)}
            onMouseLeave={() => setHoveredIndex(null)}
          >
            <div
              className="w-3 h-3 rounded-full flex-shrink-0"
              style={{ backgroundColor: alloc.color, boxShadow: `0 0 8px ${alloc.color}40` }}
            />
            <div className="flex-1 min-w-0">
              <div className="flex items-baseline gap-2">
                <span className="text-sm font-semibold text-white">{alloc.name}</span>
                <span className="text-xs text-gray-500">{alloc.amount}</span>
              </div>
              <div className="text-xs text-gray-500">{alloc.vesting}</div>
            </div>
            <span className="text-sm font-mono font-bold" style={{ color: alloc.color }}>
              {alloc.pct}%
            </span>
          </div>
        ))}
      </div>
    </div>
  );
};

export default TokenAllocationChart;
