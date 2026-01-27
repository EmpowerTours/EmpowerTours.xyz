/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/


import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Send, CheckCircle, Loader2, Terminal } from 'lucide-react';

const ContactForm: React.FC = () => {
  const [formState, setFormState] = useState({
    name: '',
    email: '',
    message: ''
  });
  const [status, setStatus] = useState<'idle' | 'submitting' | 'success'>('idle');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setStatus('submitting');
    
    // Construct email parameters
    const subject = `Incoming Transmission: ${formState.name}`;
    const body = `Identity: ${formState.name}\nComm Link: ${formState.email}\n\nPacket Data:\n${formState.message}`;
    
    // Create mailto link
    const mailtoLink = `mailto:admin@empowertours.xyz?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`;
    
    // Simulate API call / Processing delay for UX
    setTimeout(() => {
      // Open email client
      window.location.href = mailtoLink;

      setStatus('success');
      setFormState({ name: '', email: '', message: '' });
      
      // Reset status after showing success message
      setTimeout(() => {
        setStatus('idle');
      }, 5000);
    }, 1500);
  };

  return (
    <div className="w-full max-w-2xl mx-auto p-1 rounded-none bg-gradient-to-b from-white/10 to-white/0 backdrop-blur-md relative group">
      {/* Decorative corners */}
      <div className="absolute -top-1 -left-1 w-4 h-4 border-t-2 border-l-2 border-[#836EF9]"></div>
      <div className="absolute -top-1 -right-1 w-4 h-4 border-t-2 border-r-2 border-[#836EF9]"></div>
      <div className="absolute -bottom-1 -left-1 w-4 h-4 border-b-2 border-l-2 border-[#836EF9]"></div>
      <div className="absolute -bottom-1 -right-1 w-4 h-4 border-b-2 border-r-2 border-[#836EF9]"></div>

      <div className="bg-[#0a0a12] p-8 border border-white/5 relative overflow-hidden">
        {/* Scanline effect inside form */}
        <div className="absolute inset-0 bg-[linear-gradient(rgba(18,16,20,0)_50%,rgba(0,0,0,0.1)_50%)] bg-[length:100%_4px] pointer-events-none"></div>

        <div className="mb-8 flex items-center gap-2 border-b border-white/10 pb-4">
          <Terminal className="w-5 h-5 text-[#836EF9]" />
          <span className="text-xs font-mono text-[#836EF9] uppercase tracking-widest">Subspace Transmission Terminal</span>
        </div>

        <AnimatePresence mode="wait">
          {status === 'success' ? (
            <motion.div
              initial={{ opacity: 0, scale: 0.9 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.9 }}
              className="flex flex-col items-center justify-center py-12 text-center"
            >
              <div className="w-20 h-20 border-2 border-[#00f3ff] text-[#00f3ff] rounded-full flex items-center justify-center mb-6 shadow-[0_0_30px_rgba(0,243,255,0.2)]">
                <CheckCircle className="w-10 h-10" />
              </div>
              <h3 className="text-2xl font-bold text-white mb-2 font-heading">Transmission Sent</h3>
              <p className="text-gray-400 font-mono text-sm">Opening secure comms channel...</p>
            </motion.div>
          ) : (
            <motion.form
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onSubmit={handleSubmit}
              className="space-y-6 relative z-10"
            >
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-2 group/input">
                  <label htmlFor="name" className="text-[10px] font-mono text-gray-500 uppercase tracking-wider group-focus-within/input:text-[#00f3ff] transition-colors">Identity</label>
                  <input
                    type="text"
                    id="name"
                    required
                    value={formState.name}
                    onChange={e => setFormState({...formState, name: e.target.value})}
                    className="w-full bg-black/40 border border-white/10 px-4 py-3 text-white font-mono text-sm focus:outline-none focus:border-[#00f3ff] focus:bg-[#00f3ff]/5 transition-all placeholder-white/10"
                    placeholder="ENTER NAME"
                  />
                </div>
                <div className="space-y-2 group/input">
                  <label htmlFor="email" className="text-[10px] font-mono text-gray-500 uppercase tracking-wider group-focus-within/input:text-[#00f3ff] transition-colors">Comm Link</label>
                  <input
                    type="email"
                    id="email"
                    required
                    value={formState.email}
                    onChange={e => setFormState({...formState, email: e.target.value})}
                    className="w-full bg-black/40 border border-white/10 px-4 py-3 text-white font-mono text-sm focus:outline-none focus:border-[#00f3ff] focus:bg-[#00f3ff]/5 transition-all placeholder-white/10"
                    placeholder="ENTER EMAIL"
                  />
                </div>
              </div>
              
              <div className="space-y-2 group/input">
                <label htmlFor="message" className="text-[10px] font-mono text-gray-500 uppercase tracking-wider group-focus-within/input:text-[#00f3ff] transition-colors">Packet Data</label>
                <textarea
                  id="message"
                  required
                  rows={4}
                  value={formState.message}
                  onChange={e => setFormState({...formState, message: e.target.value})}
                  className="w-full bg-black/40 border border-white/10 px-4 py-3 text-white font-mono text-sm focus:outline-none focus:border-[#00f3ff] focus:bg-[#00f3ff]/5 transition-all placeholder-white/10 resize-none"
                  placeholder="INPUT MESSAGE PARAMETERS..."
                />
              </div>

              <button
                type="submit"
                disabled={status === 'submitting'}
                className="w-full bg-[#836EF9] hover:bg-[#6d5ae0] text-white font-bold uppercase tracking-[0.2em] py-4 transition-all disabled:opacity-50 flex items-center justify-center gap-2 group relative overflow-hidden"
                data-hover="true"
              >
                <span className="relative z-10 flex items-center gap-2">
                  {status === 'submitting' ? (
                    <>
                      <Loader2 className="w-5 h-5 animate-spin" /> Encrypting...
                    </>
                  ) : (
                    <>
                      Transmit Data <Send className="w-4 h-4 group-hover:translate-x-2 transition-transform" />
                    </>
                  )}
                </span>
                {/* Button Glitch effect overlay */}
                <div className="absolute inset-0 bg-white/20 translate-y-full group-hover:translate-y-0 transition-transform duration-300 skew-y-12"></div>
              </button>
            </motion.form>
          )}
        </AnimatePresence>
      </div>
    </div>
  );
};

export default ContactForm;