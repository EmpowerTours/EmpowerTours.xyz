
/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/


import React, { useEffect, useState } from 'react';
import { motion, useSpring, useMotionValue } from 'framer-motion';

const CustomCursor: React.FC = () => {
  const [isHovering, setIsHovering] = useState(false);
  const [isVisible, setIsVisible] = useState(false);
  
  const mouseX = useMotionValue(0);
  const mouseY = useMotionValue(0);
  
  // Smooth spring animation for the trailing ring
  const springConfig = { damping: 25, stiffness: 400, mass: 0.2 }; 
  const ringX = useSpring(mouseX, springConfig);
  const ringY = useSpring(mouseY, springConfig);

  useEffect(() => {
    // Check if device is touch-enabled to disable custom cursor
    const isTouchDevice = window.matchMedia("(pointer: coarse)").matches;
    if (isTouchDevice) return;

    const updateMousePosition = (e: MouseEvent) => {
      if (!isVisible) setIsVisible(true);
      mouseX.set(e.clientX);
      mouseY.set(e.clientY);

      const target = e.target as HTMLElement;
      // Check for interactive elements
      const clickable = target.closest('button') || 
                        target.closest('a') || 
                        target.closest('input') ||
                        target.closest('textarea') ||
                        target.closest('[data-hover="true"]');
      setIsHovering(!!clickable);
    };

    window.addEventListener('mousemove', updateMousePosition, { passive: true });
    return () => window.removeEventListener('mousemove', updateMousePosition);
  }, [mouseX, mouseY, isVisible]);

  if (!isVisible) return null;

  return (
    <div className="fixed top-0 left-0 z-[9999] pointer-events-none mix-blend-screen hidden md:block">
      
      {/* Central Crosshair - Follows mouse exactly */}
      <motion.div
        className="absolute top-0 left-0 flex items-center justify-center"
        style={{ x: mouseX, y: mouseY, translateX: '-50%', translateY: '-50%' }}
      >
         <div className={`w-1 h-1 bg-white rounded-full transition-colors duration-200 ${isHovering ? 'bg-[#00f3ff]' : ''}`} />
         <div className={`absolute w-4 h-4 border-[0.5px] border-white/50 transition-all duration-200 ${isHovering ? 'scale-0 opacity-0' : 'scale-100 opacity-100'}`} />
      </motion.div>

      {/* Trailing Ring / Targeting System */}
      <motion.div
        className="absolute top-0 left-0 flex items-center justify-center"
        style={{ x: ringX, y: ringY, translateX: '-50%', translateY: '-50%' }}
      >
        <motion.div
          className="relative flex items-center justify-center border border-[#836EF9]"
          animate={{
            width: isHovering ? 40 : 24,
            height: isHovering ? 40 : 24,
            rotate: isHovering ? 45 : 0,
            borderColor: isHovering ? '#00f3ff' : 'rgba(131,110,249, 0.5)',
          }}
          transition={{ type: "spring", stiffness: 300, damping: 20 }}
        >
          {/* Corner markers for sci-fi feel */}
          <div className="absolute -top-[1px] -left-[1px] w-2 h-2 border-t border-l border-current" />
          <div className="absolute -bottom-[1px] -right-[1px] w-2 h-2 border-b border-r border-current" />
          
          {/* Hover Text Label */}
          <motion.span 
            className="absolute top-full left-1/2 mt-2 text-[8px] font-mono font-bold uppercase tracking-widest text-[#00f3ff] whitespace-nowrap"
            initial={{ opacity: 0, x: '-50%' }}
            animate={{ 
              opacity: isHovering ? 1 : 0,
              y: isHovering ? 4 : 0
            }}
            style={{ x: '-50%' }}
          >
            ACCESS
          </motion.span>
        </motion.div>
      </motion.div>
    </div>
  );
};

export default CustomCursor;
