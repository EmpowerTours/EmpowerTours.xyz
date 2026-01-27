/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/


import React, { useMemo } from 'react';
import { motion } from 'framer-motion';

const StarField = () => {
  // Increased star count for a dense universe feel
  const stars = useMemo(() => {
    return Array.from({ length: 50 }).map((_, i) => ({
      id: i,
      size: Math.random() * 1.5 + 0.5,
      x: Math.random() * 100,
      y: Math.random() * 100,
      duration: Math.random() * 3 + 1, // Faster twinkle
      delay: Math.random() * 2,
      opacity: Math.random() * 0.8 + 0.2
    }));
  }, []);

  return (
    <div className="absolute inset-0 z-0 pointer-events-none">
      {stars.map((star) => (
        <motion.div
          key={star.id}
          className="absolute rounded-full bg-white will-change-[opacity]"
          style={{
            left: `${star.x}%`,
            top: `${star.y}%`,
            width: star.size,
            height: star.size,
            boxShadow: `0 0 ${star.size * 2}px white`
          }}
          initial={{ opacity: star.opacity }}
          animate={{
            opacity: [star.opacity, 1, star.opacity],
            scale: [1, 1.2, 1],
          }}
          transition={{
            duration: star.duration,
            repeat: Infinity,
            ease: "easeInOut",
            delay: star.delay,
          }}
        />
      ))}
    </div>
  );
};

const FluidBackground: React.FC = () => {
  return (
    <div className="fixed inset-0 -z-10 overflow-hidden bg-[#020205]">
      
      <StarField />

      {/* Nebula 1: Monad Purple - Uses plus-lighter for light addition effect (glowing) */}
      <motion.div
        className="absolute top-[-20%] left-[10%] w-[60vw] h-[60vw] bg-[#836EF9] rounded-full filter blur-[100px] opacity-20 will-change-transform mix-blend-screen"
        animate={{
          x: [0, 30, -30, 0],
          scale: [1, 1.1, 1],
          opacity: [0.2, 0.3, 0.2]
        }}
        transition={{
          duration: 20,
          repeat: Infinity,
          ease: "easeInOut"
        }}
      />

      {/* Nebula 2: Deep Cosmic Blue */}
      <motion.div
        className="absolute bottom-[-10%] right-[-10%] w-[70vw] h-[70vw] bg-[#2a2b8a] rounded-full filter blur-[120px] opacity-20 will-change-transform mix-blend-screen"
        animate={{
          y: [0, -40, 0],
          scale: [1, 1.2, 1],
        }}
        transition={{
          duration: 25,
          repeat: Infinity,
          ease: "easeInOut"
        }}
      />

      {/* Nebula 3: Cyan Event Horizon */}
      <motion.div
        className="absolute top-[40%] left-[40%] w-[40vw] h-[40vw] bg-[#00f3ff] rounded-full filter blur-[80px] opacity-10 will-change-transform mix-blend-screen"
        animate={{
          x: [0, -50, 50, 0],
          y: [0, 50, -50, 0],
        }}
        transition={{
          duration: 30,
          repeat: Infinity,
          ease: "linear"
        }}
      />

      {/* Scanline/Grid Overlay for "Tech" feel */}
      <div className="absolute inset-0 bg-[linear-gradient(rgba(18,16,20,0)_50%,rgba(0,0,0,0.25)_50%),linear-gradient(90deg,rgba(255,0,0,0.06),rgba(0,255,0,0.02),rgba(0,0,255,0.06))] z-[1] bg-[length:100%_4px,6px_100%] pointer-events-none opacity-20"></div>
      
      {/* Vignette */}
      <div className="absolute inset-0 bg-radial-gradient from-transparent via-[#020205]/50 to-[#020205] pointer-events-none z-[2]" />
    </div>
  );
};

export default FluidBackground;