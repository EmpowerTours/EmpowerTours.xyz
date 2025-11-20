/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/


import React from 'react';
import { motion } from 'framer-motion';
import { Project } from '../types';
import { ArrowUpRight, Disc } from 'lucide-react';

interface ProjectCardProps {
  project: Project;
  onClick: () => void;
}

const ProjectCard: React.FC<ProjectCardProps> = ({ project, onClick }) => {
  return (
    <motion.div
      className="group relative h-[420px] w-full overflow-hidden border border-white/10 bg-[#05050a] cursor-pointer transition-colors hover:border-[#836EF9]/50"
      initial="rest"
      whileHover="hover"
      whileTap="hover"
      animate="rest"
      data-hover="true"
      onClick={onClick}
    >
      {/* Top Tech Bar Decoration */}
      <div className="absolute top-0 left-0 w-full h-1 bg-white/5 z-20">
        <motion.div 
          className="h-full bg-[#836EF9]" 
          variants={{
            rest: { width: '0%' },
            hover: { width: '100%' }
          }}
          transition={{ duration: 0.4 }}
        />
      </div>

      {/* Image Background with Glitch/Zoom */}
      <div className="absolute inset-0 overflow-hidden">
        <motion.img 
          src={project.image} 
          alt={project.name} 
          className="h-full w-full object-cover opacity-60 grayscale mix-blend-luminosity will-change-transform"
          variants={{
            rest: { scale: 1, filter: 'grayscale(100%) brightness(0.6)' },
            hover: { scale: 1.1, filter: 'grayscale(0%) brightness(0.8)' }
          }}
          transition={{ duration: 0.6, ease: [0.33, 1, 0.68, 1] }}
        />
        {/* Gradient Overlay */}
        <div className="absolute inset-0 bg-gradient-to-t from-[#020205] via-[#020205]/80 to-transparent" />
        
        {/* Grid Overlay */}
        <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-20" />
      </div>

      {/* Corner Decorations */}
      <div className="absolute top-4 right-4 z-20 w-2 h-2 border-t border-r border-white/30" />
      <div className="absolute bottom-4 left-4 z-20 w-2 h-2 border-b border-l border-white/30" />

      {/* Overlay Info */}
      <div className="absolute inset-0 p-8 flex flex-col justify-between z-10">
        <div className="flex justify-between items-start">
           <motion.span 
             className="text-[10px] font-mono text-[#00f3ff] uppercase tracking-widest border border-[#00f3ff]/30 px-2 py-1 backdrop-blur-sm"
           >
             {project.category}
           </motion.span>
           
           <motion.div
             variants={{
               rest: { opacity: 0, scale: 0.8 },
               hover: { opacity: 1, scale: 1 }
             }}
             className="bg-[#836EF9] text-white p-2"
           >
             <ArrowUpRight className="w-5 h-5" />
           </motion.div>
        </div>

        <div className="relative">
          <div className="overflow-hidden mb-2">
            <motion.h3 
              className="font-heading text-3xl font-bold text-white uppercase tracking-tight"
              variants={{
                rest: { y: 0 },
                hover: { x: 5 }
              }}
              transition={{ duration: 0.3 }}
            >
              {project.name}
            </motion.h3>
          </div>
          
          <motion.div
            className="w-full h-px bg-white/20 mb-4 origin-left"
            variants={{
              rest: { scaleX: 0.2 },
              hover: { scaleX: 1 }
            }}
          />

          <motion.p 
             className="text-sm text-gray-400 font-mono leading-relaxed line-clamp-2"
             variants={{
              rest: { opacity: 0.7 },
              hover: { opacity: 1, color: '#fff' }
            }}
          >
            {project.description}
          </motion.p>

          <motion.div
            className="mt-4 flex items-center gap-2 text-[10px] font-mono text-[#836EF9] uppercase"
            variants={{
              rest: { opacity: 0, y: 10 },
              hover: { opacity: 1, y: 0 }
            }}
          >
             <Disc className="w-3 h-3 animate-spin" /> System Online
          </motion.div>
        </div>
      </div>
    </motion.div>
  );
};

export default ProjectCard;