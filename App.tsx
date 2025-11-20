
/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/


import React, { useState, useEffect } from 'react';
import { motion, useScroll, useTransform, AnimatePresence } from 'framer-motion';
import { Menu, X, ChevronLeft, ChevronRight, Code, Layers, Cpu, Rocket, Globe, Zap, ExternalLink, Sparkles, Orbit, ArrowLeft, Smartphone, Bot } from 'lucide-react';
import FluidBackground from './components/FluidBackground';
import GradientText from './components/GlitchText';
import CustomCursor from './components/CustomCursor';
import ProjectCard from './components/ProjectCard';
import ContactForm from './components/ContactForm';
import AIChat from './components/AIChat';
import { Project, Service } from './types';

// Portfolio Data - Updated with detailed specs
const PROJECTS: Project[] = [
  { 
    id: '1', 
    name: 'Monad Odyssey', 
    category: 'Ecosystem Galaxy', 
    url: 'https://monad-universe-production.up.railway.app/',
    // Image: 3D Galaxy / Network
    image: 'https://images.unsplash.com/photo-1462331940025-496dfbfc7564?q=80&w=2022&auto=format&fit=crop',
    description: 'An immersive 3D galaxy visualization of the Monad ecosystem. Explore 100+ projects across DeFi, NFTs, Gaming, and Infrastructure as an interactive cosmic universe. Features real-time filtering, AI smart suggestions, and deep project discovery.',
    features: [
      '3D Spiral Galaxy Visualization',
      'Real-time Ecosystem Filtering',
      'AI-Powered Smart Suggestions',
      'Wallet Integration (Privy)'
    ],
    technologies: [
      'Monad Blockchain',
      'React Three Fiber',
      'Three.js',
      'Privy'
    ]
  },
  { 
    id: '2', 
    name: 'EmpowerTours Bot', 
    category: 'AI Adventure Companion', 
    url: 'https://t.me/AI_RobotExpert_bot',
    // Image: Working Cyberpunk Robot
    image: 'https://images.unsplash.com/photo-1531746790731-6c087fecd65a?q=80&w=2000&auto=format&fit=crop',
    description: 'Web3-Powered Rock Climbing Adventures & Community on Telegram. Log ascents, build routes, and compete in tournaments while earning $TOURS tokens. Features a natural language interface for seamless blockchain interaction on Monad Testnet.',
    features: [
      'Journal Climbs & Earn Rewards',
      'Build & Trade Route NFTs',
      'Community Tournaments',
      'Gasless Bot Transactions'
    ],
    technologies: [
      'Python / Aiogram',
      'Monad Testnet',
      'FastAPI',
      'Solidity'
    ]
  },
  { 
    id: '3', 
    name: 'Empower MiniApp', 
    category: 'Farcaster Social Hub', 
    url: 'https://farcaster.xyz/miniapps/83hgtZau7TNB/empowertours',
    // Image: Purple Decentralized Network / Social Graph (Farcaster Themed)
    image: 'https://images.unsplash.com/photo-1614680376593-902f74cf0d41?q=80&w=2000&auto=format&fit=crop',
    description: 'A comprehensive Web3 platform built as a Farcaster Mini App. Mint Travel Passport NFTs for 195 countries, license Music NFTs, and engage in social experiences with gasless transactions powered by Account Abstraction.',
    features: [
      'Travel Passport NFTs',
      'Music NFT Licensing',
      'Auto-Farcaster Casting',
      'Gasless (Safe + Pimlico)'
    ],
    technologies: [
      'Monad',
      'Farcaster Frames v2',
      'Envio Indexer',
      'Safe AA'
    ]
  },
];

const SERVICES: Service[] = [
  {
    title: 'Smart Contract Engineering',
    description: 'Secure, gas-optimized Solidity contracts tailored for the high-throughput Monad architecture.',
    icon: Code
  },
  {
    title: 'Hyper-Scale dApps',
    description: 'Full-stack decentralized applications with seamless UI/UX and robust wallet integration.',
    icon: Layers
  },
  {
    title: 'Bot & MiniApp Dev',
    description: 'Building bridge-less interactions via Telegram Bots and Farcaster MiniApps using Account Abstraction.',
    icon: Bot
  }
];

const App: React.FC = () => {
  const { scrollYProgress } = useScroll();
  const y = useTransform(scrollYProgress, [0, 1], [0, -100]);
  const opacity = useTransform(scrollYProgress, [0, 0.2], [1, 0]);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [selectedProject, setSelectedProject] = useState<Project | null>(null);

  // Handle keyboard navigation for modal
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!selectedProject) return;
      if (e.key === 'ArrowLeft') navigateProject('prev');
      if (e.key === 'ArrowRight') navigateProject('next');
      if (e.key === 'Escape') setSelectedProject(null);
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [selectedProject]);

  // Prevent scrolling when modal is open
  useEffect(() => {
    if (selectedProject || mobileMenuOpen) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = 'unset';
    }
    return () => { document.body.style.overflow = 'unset'; };
  }, [selectedProject, mobileMenuOpen]);

  const scrollToSection = (id: string) => {
    setMobileMenuOpen(false);
    const element = document.getElementById(id);
    if (element) {
      const headerOffset = 80;
      const elementPosition = element.getBoundingClientRect().top;
      const offsetPosition = elementPosition + window.scrollY - headerOffset;

      window.scrollTo({
        top: offsetPosition,
        behavior: 'smooth'
      });
    }
  };

  const navigateProject = (direction: 'next' | 'prev') => {
    if (!selectedProject) return;
    const currentIndex = PROJECTS.findIndex(p => p.id === selectedProject.id);
    let nextIndex;
    if (direction === 'next') {
      nextIndex = (currentIndex + 1) % PROJECTS.length;
    } else {
      nextIndex = (currentIndex - 1 + PROJECTS.length) % PROJECTS.length;
    }
    setSelectedProject(PROJECTS[nextIndex]);
  };
  
  return (
    <div className="relative min-h-screen text-white selection:bg-[#836EF9] selection:text-white cursor-auto md:cursor-none overflow-x-hidden font-sans">
      <CustomCursor />
      <FluidBackground />
      <AIChat />
      
      {/* Navigation */}
      <nav className="fixed top-0 left-0 right-0 z-40 flex items-center justify-between px-4 md:px-10 py-4 md:py-6 border-b border-white/5 bg-[#020205]/80 md:bg-black/20 backdrop-blur-xl">
        <div className="font-heading text-lg md:text-2xl font-bold tracking-tighter text-white cursor-default z-50 flex items-center gap-3">
          <div className="relative w-8 h-8 md:w-10 md:h-10 flex items-center justify-center">
            <div className="absolute inset-0 bg-[#836EF9] rounded-lg blur opacity-50 animate-pulse"></div>
            <div className="relative w-full h-full bg-[#1a1b3b] border border-[#836EF9] rounded-lg flex items-center justify-center">
               <Rocket className="w-4 h-4 md:w-5 md:h-5 text-white transform -rotate-45" />
            </div>
          </div>
          <span className="bg-clip-text text-transparent bg-gradient-to-r from-white to-gray-400">EmpowerTours</span>
        </div>
        
        {/* Desktop Menu */}
        <div className="hidden md:flex gap-10 text-xs font-bold tracking-[0.2em] uppercase">
          {['Services', 'Portfolio', 'Contact'].map((item) => (
            <button 
              key={item} 
              onClick={() => scrollToSection(item.toLowerCase())}
              className="hover:text-[#836EF9] transition-colors text-white cursor-pointer bg-transparent border-none relative group"
              data-hover="true"
            >
              <span className="absolute -left-4 top-1/2 -translate-y-1/2 w-1.5 h-1.5 bg-[#836EF9] rounded-full opacity-0 group-hover:opacity-100 transition-opacity"></span>
              {item}
            </button>
          ))}
        </div>
        
        {/* Mobile Menu Toggle */}
        <button 
          className="md:hidden text-white z-50 relative w-10 h-10 flex items-center justify-center bg-white/5 rounded-lg border border-white/10 active:scale-95 transition-transform"
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          aria-label="Toggle menu"
        >
           {mobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
        </button>
      </nav>

      {/* Mobile Menu Overlay */}
      <AnimatePresence>
        {mobileMenuOpen && (
          <motion.div
            initial={{ opacity: 0, x: '100%' }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: '100%' }}
            transition={{ type: "spring", bounce: 0, duration: 0.4 }}
            className="fixed inset-0 z-30 bg-[#020205] flex flex-col items-center justify-center gap-8 md:hidden pt-20"
          >
             <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-10 mix-blend-overlay pointer-events-none"></div>
            {['Services', 'Portfolio', 'Contact'].map((item) => (
              <button
                key={item}
                onClick={() => scrollToSection(item.toLowerCase())}
                className="text-3xl font-heading font-bold text-white hover:text-[#836EF9] transition-colors uppercase bg-transparent border-none tracking-widest w-full py-4 text-center"
              >
                {item}
              </button>
            ))}
          </motion.div>
        )}
      </AnimatePresence>

      {/* HERO SECTION */}
      <header className="relative h-[100svh] min-h-[500px] flex flex-col items-center justify-center overflow-hidden px-4 pt-16 md:pt-20">
        <motion.div 
          style={{ y, opacity }}
          className="z-10 text-center flex flex-col items-center w-full max-w-7xl"
        >
           {/* Badge */}
          <motion.div
            initial={{ opacity: 0, scale: 0.5 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 1, delay: 0.2 }}
            className="flex items-center gap-2 md:gap-3 text-[10px] md:text-xs font-mono text-[#00f3ff] tracking-[0.2em] md:tracking-[0.3em] uppercase mb-6 md:mb-8 bg-[#00f3ff]/5 border border-[#00f3ff]/20 px-4 py-2 md:px-6 md:py-2 rounded-full backdrop-blur-md shadow-[0_0_20px_rgba(0,243,255,0.1)] max-w-[90vw] truncate"
          >
            <span className="truncate">Architecting the Monad Verse</span>
          </motion.div>

          {/* Main Title */}
          <div className="relative w-full flex flex-col justify-center items-center perspective-[1000px]">
            <h1 className="text-[14vw] md:text-[9vw] leading-[0.9] md:leading-[0.8] font-heading font-bold tracking-tighter text-center mb-2 mix-blend-lighten opacity-90">
              BEYOND
            </h1>
             <GradientText 
              text="LIMITS" 
              as="h1" 
              className="text-[14vw] md:text-[9vw] leading-[0.9] md:leading-[0.8] font-heading font-bold tracking-tighter text-center drop-shadow-[0_0_30px_rgba(131,110,249,0.5)]" 
            />
          </div>
          
          <motion.div
             initial={{ width: 0 }}
             animate={{ width: "100%" }}
             transition={{ duration: 1.5, delay: 0.5, ease: "circOut" }}
             className="w-full max-w-[200px] md:max-w-xs h-[1px] bg-gradient-to-r from-transparent via-[#00f3ff] to-transparent mt-6 md:mt-10 mb-6 md:mb-10"
          />

          <motion.p
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.8, duration: 1 }}
            className="text-base md:text-2xl font-light max-w-3xl mx-auto text-gray-300 leading-relaxed px-4"
          >
            We build next-generation infrastructure and applications on the <span className="text-[#836EF9] font-semibold glow-text">Monad Blockchain</span>. High throughput. Parallel execution. Infinite scale.
          </motion.p>

          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 1, duration: 1 }}
            className="mt-8 md:mt-12 flex flex-col md:flex-row gap-4 md:gap-6 w-full md:w-auto px-6 md:px-0"
          >
            <button 
              onClick={() => scrollToSection('portfolio')}
              className="w-full md:w-auto px-8 py-4 bg-[#836EF9] text-white md:rounded-none rounded-lg md:skew-x-[-10deg] font-bold tracking-widest uppercase text-sm hover:bg-[#6d5ae0] transition-all hover:skew-x-0 shadow-[0_0_30px_rgba(131,110,249,0.3)] border-r-0 md:border-r-2 border-b-2 border-[#5b4ad6]"
              data-hover="true"
            >
              <span className="block md:skew-x-[10deg]">Explore Projects</span>
            </button>
            <button 
              onClick={() => scrollToSection('contact')}
              className="w-full md:w-auto px-8 py-4 bg-transparent border border-white/20 text-white md:rounded-none rounded-lg md:skew-x-[-10deg] font-bold tracking-widest uppercase text-sm hover:border-[#00f3ff] hover:text-[#00f3ff] hover:shadow-[0_0_20px_rgba(0,243,255,0.2)] transition-all"
              data-hover="true"
            >
              <span className="block md:skew-x-[10deg]">Initiate Contact</span>
            </button>
          </motion.div>
        </motion.div>
      </header>

      {/* SERVICES SECTION */}
      <section id="services" className="relative z-10 py-16 md:py-32 bg-black/30 backdrop-blur-sm border-y border-white/5">
        <div className="max-w-7xl mx-auto px-4 md:px-6">
          <div className="mb-12 md:mb-16 text-center">
             <h2 className="text-3xl md:text-6xl font-heading font-bold uppercase leading-tight mb-4 text-transparent bg-clip-text bg-gradient-to-b from-white to-gray-600">
               Mission <span className="text-[#00f3ff]">Capabilities</span>
            </h2>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6 md:gap-8">
            {SERVICES.map((service, index) => (
              <motion.div
                key={index}
                whileHover={{ y: -10, boxShadow: "0 20px 40px -10px rgba(131,110,249,0.1)" }}
                className="p-6 md:p-8 border border-white/5 bg-[#0a0a12] relative overflow-hidden group rounded-lg md:rounded-none"
              >
                <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-[#836EF9] to-[#00f3ff] transform scale-x-0 group-hover:scale-x-100 transition-transform duration-500 origin-left"></div>
                
                <div className="w-12 h-12 md:w-14 md:h-14 rounded-full bg-[#836EF9]/10 flex items-center justify-center mb-6 group-hover:bg-[#836EF9]/20 transition-colors duration-500 border border-[#836EF9]/20">
                  <service.icon className="w-6 h-6 md:w-7 md:h-7 text-[#836EF9] group-hover:text-white transition-colors" />
                </div>
                <h3 className="text-lg md:text-xl font-bold font-heading mb-3 text-white tracking-wide">{service.title}</h3>
                <p className="text-gray-400 leading-relaxed text-sm font-mono">
                  {service.description}
                </p>
              </motion.div>
            ))}
          </div>
        </div>
      </section>

      {/* PORTFOLIO SECTION */}
      <section id="portfolio" className="relative z-10 py-16 md:py-32">
        <div className="max-w-[1600px] mx-auto px-4 md:px-6">
          <div className="flex flex-col md:flex-row justify-between items-end mb-10 md:mb-16 px-2 md:px-4">
             <div className="max-w-2xl">
              <div className="flex items-center gap-2 text-[#00f3ff] mb-2 font-mono text-xs md:text-sm">
                <Sparkles className="w-4 h-4" /> <span>DEPLOYED SYSTEMS</span>
              </div>
              <h2 className="text-4xl md:text-7xl font-heading font-bold uppercase leading-[0.9] drop-shadow-lg mb-4">
                Galactic <br/> 
                <span className="text-transparent bg-clip-text bg-gradient-to-r from-[#836EF9] to-[#00f3ff]">Portfolio</span>
              </h2>
             </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 md:gap-8">
            {PROJECTS.map((project) => (
              <ProjectCard key={project.id} project={project} onClick={() => setSelectedProject(project)} />
            ))}
          </div>
        </div>
      </section>

      {/* CONTACT SECTION */}
      <section id="contact" className="relative z-10 py-16 md:py-32 px-4 md:px-6 bg-gradient-to-b from-transparent to-[#05050a]">
        <div className="max-w-4xl mx-auto text-center">
          <div className="mb-10 md:mb-16">
             <h2 className="text-3xl md:text-6xl font-heading font-bold text-white mb-4 md:mb-6">
               Join The <br/> <span className="text-[#836EF9]">Monad Revolution</span>
             </h2>
             <p className="text-gray-300 text-base md:text-lg px-4">
               Open a channel with our engineering team.
             </p>
          </div>
          
          <ContactForm />
        </div>
      </section>

      <footer className="relative z-10 border-t border-white/5 py-12 bg-[#020205]">
        <div className="max-w-7xl mx-auto px-6 flex flex-col md:flex-row justify-between items-center gap-8 text-center md:text-left">
          <div className="flex items-center gap-2 justify-center md:justify-start">
             <Rocket className="w-6 h-6 text-[#836EF9]" />
             <div className="font-heading text-xl font-bold tracking-tighter text-white">EmpowerTours</div>
          </div>
          
          <div className="text-xs text-gray-500 font-mono order-3 md:order-2">
            © 2025 EmpowerTours. Built on Monad.
          </div>
          
          <div className="flex gap-6 md:gap-8 order-2 md:order-3">
            <a href="https://x.com/EmpowerTours" target="_blank" rel="noopener noreferrer" className="text-gray-400 hover:text-[#00f3ff] transition-colors uppercase text-xs tracking-widest p-2">X (Twitter)</a>
            <a href="https://warpcast.com/empowertoursbot" target="_blank" rel="noopener noreferrer" className="text-gray-400 hover:text-[#00f3ff] transition-colors uppercase text-xs tracking-widest p-2">Farcaster</a>
          </div>
        </div>
      </footer>

      {/* Project Detail Modal */}
      <AnimatePresence>
        {selectedProject && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={() => setSelectedProject(null)}
            className="fixed inset-0 z-[60] flex items-center justify-center md:p-4 bg-black/95 backdrop-blur-xl overflow-hidden"
          >
            <motion.div
              initial={{ scale: 0.95, opacity: 0, y: 20 }}
              animate={{ scale: 1, opacity: 1, y: 0 }}
              exit={{ scale: 0.95, opacity: 0, y: 20 }}
              onClick={(e) => e.stopPropagation()}
              className="relative w-full h-full md:h-auto md:max-h-[90vh] md:max-w-6xl bg-[#0a0a12] md:border border-white/10 md:rounded-none flex flex-col md:flex-row shadow-[0_0_50px_rgba(131,110,249,0.2)]"
            >
              {/* Mobile Header with Close Button */}
              <div className="md:hidden fixed top-0 left-0 right-0 z-50 flex justify-between items-center p-4 bg-black/80 backdrop-blur-md border-b border-white/10">
                <button onClick={() => setSelectedProject(null)} className="flex items-center gap-2 text-white/80 font-mono text-xs uppercase">
                   <ArrowLeft className="w-4 h-4" /> Back
                </button>
                <span className="font-heading font-bold text-white truncate max-w-[50%]">{selectedProject.name}</span>
                <button onClick={() => setSelectedProject(null)} className="p-2 bg-white/10 rounded-full">
                   <X className="w-4 h-4 text-white" />
                </button>
              </div>

              {/* Close Button (Desktop) */}
              <button
                onClick={() => setSelectedProject(null)}
                className="absolute top-4 right-4 z-30 p-2 bg-black/50 text-white hover:bg-[#836EF9] transition-colors border border-white/10 hidden md:block"
                data-hover="true"
              >
                <X className="w-6 h-6" />
              </button>

              {/* Navigation Buttons (Desktop) */}
              <button
                onClick={(e) => { e.stopPropagation(); navigateProject('prev'); }}
                className="absolute left-4 top-1/2 -translate-y-1/2 z-30 p-3 bg-black/50 text-white hover:bg-[#836EF9] transition-colors border border-white/10 backdrop-blur-sm hidden md:block"
                data-hover="true"
              >
                <ChevronLeft className="w-6 h-6" />
              </button>

              <button
                onClick={(e) => { e.stopPropagation(); navigateProject('next'); }}
                className="absolute right-4 top-1/2 -translate-y-1/2 z-30 p-3 bg-black/50 text-white hover:bg-[#836EF9] transition-colors border border-white/10 backdrop-blur-sm hidden md:block"
                data-hover="true"
              >
                <ChevronRight className="w-6 h-6" />
              </button>

              {/* Image Side */}
              <div className="w-full md:w-1/2 h-[40vh] md:h-auto relative overflow-hidden bg-black border-b md:border-b-0 md:border-r border-white/10 mt-14 md:mt-0 shrink-0">
                <AnimatePresence mode="wait">
                  <motion.img 
                    key={selectedProject.id}
                    src={selectedProject.image} 
                    alt={selectedProject.name} 
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.4 }}
                    className="absolute inset-0 w-full h-full object-cover opacity-80"
                  />
                </AnimatePresence>
                <div className="absolute inset-0 bg-gradient-to-t from-[#0a0a12] via-transparent to-transparent" />
                {/* Tech lines overlay */}
                <div className="absolute top-0 left-0 w-full h-full bg-[linear-gradient(transparent_2px,black_2px)] bg-[length:100%_4px] opacity-20 pointer-events-none"></div>
              </div>

              {/* Content Side */}
              <div className="w-full md:w-1/2 p-6 md:p-12 overflow-y-auto custom-scrollbar bg-[#0a0a12] h-full md:h-auto pb-20 md:pb-12">
                <motion.div
                  key={selectedProject.id}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: 0.4, delay: 0.1 }}
                >
                  <div className="flex items-center gap-3 text-[#00f3ff] mb-4 md:mb-6 border-b border-white/10 pb-4">
                     <Globe className="w-4 h-4" />
                     <span className="font-mono text-xs md:text-sm tracking-[0.2em] uppercase">{selectedProject.category}</span>
                  </div>
                  
                  <h3 className="text-3xl md:text-5xl font-heading font-bold leading-none mb-4 md:mb-6 text-white">
                    {selectedProject.name}
                  </h3>
                  
                  <p className="text-gray-300 leading-relaxed text-base md:text-lg font-light mb-8 md:mb-10">
                    {selectedProject.description}
                  </p>
                  
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-8 md:gap-10 mb-8 md:mb-10">
                    <div>
                      <h4 className="text-xs font-bold uppercase tracking-widest text-gray-500 mb-4 flex items-center gap-2">
                        <Zap className="w-3 h-3 text-[#836EF9]" /> System Capabilities
                      </h4>
                      <ul className="space-y-3">
                        {selectedProject.features.map((feature, i) => (
                          <li key={i} className="text-sm text-gray-300 flex items-start gap-3 font-mono">
                            <span className="w-1.5 h-1.5 bg-[#00f3ff] mt-1.5 shrink-0 rounded-full" />
                            {feature}
                          </li>
                        ))}
                      </ul>
                    </div>
                    <div>
                      <h4 className="text-xs font-bold uppercase tracking-widest text-gray-500 mb-4 flex items-center gap-2">
                        <Code className="w-3 h-3 text-[#836EF9]" /> Tech Architecture
                      </h4>
                      <div className="flex flex-wrap gap-2">
                        {selectedProject.technologies.map((tech, i) => (
                          <span key={i} className="text-[10px] font-mono uppercase tracking-wider px-3 py-1 bg-white/5 border border-white/10 text-[#836EF9] rounded-sm">
                            {tech}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>

                  <a 
                    href={selectedProject.url} 
                    target="_blank" 
                    rel="noopener noreferrer"
                    className="inline-flex items-center justify-center gap-3 w-full py-4 bg-[#836EF9] hover:bg-[#6d5ae0] active:scale-[0.98] text-white font-bold uppercase tracking-[0.2em] text-sm transition-all md:hover:shadow-[0_0_30px_rgba(131,110,249,0.4)] group border border-transparent md:hover:border-[#fff] rounded-lg md:rounded-none"
                    data-hover="true"
                  >
                    Initialize System <ExternalLink className="w-4 h-4 group-hover:translate-x-1 group-hover:-translate-y-1 transition-transform" />
                  </a>
                </motion.div>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
};

export default App;
