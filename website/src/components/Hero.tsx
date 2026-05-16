import { motion, useScroll, useTransform } from 'framer-motion'
import { useRef } from 'react'
import { ArrowRight } from 'lucide-react'
import { GithubIcon } from './icons'

export default function Hero() {
  const ref = useRef(null)
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ['start start', 'end start'],
  })
  const y = useTransform(scrollYProgress, [0, 1], [0, 200])
  const opacity = useTransform(scrollYProgress, [0, 0.8], [1, 0])

  return (
    <section ref={ref} className="relative min-h-screen flex items-center justify-center overflow-hidden pt-16">
      {/* Animated background */}
      <div className="absolute inset-0">
        {/* Gradient orbs */}
        <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-brand-purple/20 rounded-full blur-[128px] animate-pulse-glow" />
        <div className="absolute bottom-1/4 right-1/4 w-96 h-96 bg-brand-blue/15 rounded-full blur-[128px] animate-pulse-glow" style={{ animationDelay: '2s' }} />
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-brand-cyan/5 rounded-full blur-[128px] animate-float-slow" />

        {/* Film strip decorations */}
        <div className="absolute top-20 left-10 opacity-5">
          <FilmStrip />
        </div>
        <div className="absolute bottom-20 right-10 opacity-5">
          <FilmStrip />
        </div>

        {/* Grid pattern */}
        <div
          className="absolute inset-0 opacity-[0.03]"
          style={{
            backgroundImage: `linear-gradient(rgba(139,92,246,0.3) 1px, transparent 1px), linear-gradient(90deg, rgba(139,92,246,0.3) 1px, transparent 1px)`,
            backgroundSize: '60px 60px',
          }}
        />
      </div>

      <motion.div style={{ y, opacity }} className="relative z-10 text-center px-4 max-w-5xl mx-auto">
        {/* Logo with glow */}
        <motion.div
          initial={{ scale: 0, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ duration: 0.8, type: 'spring', stiffness: 100 }}
          className="mb-8 inline-block"
        >
          <div className="relative">
            <div className="absolute inset-0 blur-2xl bg-brand-purple/30 rounded-full scale-150 animate-pulse-glow" />
            <img src="/loom-logo.png" alt="Loom" className="relative w-24 h-24 sm:w-32 sm:h-32 mx-auto" />
          </div>
        </motion.div>

        {/* Pre-alpha badge */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
          className="mb-6"
        >
          <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium bg-brand-purple/10 border border-brand-purple/30 text-brand-purple">
            <span className="w-2 h-2 bg-brand-purple rounded-full animate-pulse" />
            Pre-Alpha — Actively Developing
          </span>
        </motion.div>

        {/* Main heading */}
        <motion.h1
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4, duration: 0.8 }}
          className="text-4xl sm:text-6xl lg:text-7xl font-bold tracking-tight mb-6"
        >
          <span className="text-gradient">Unified Media</span>
          <br />
          <span className="text-white">Automation</span>
        </motion.h1>

        {/* Subtitle */}
        <motion.p
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.6 }}
          className="text-lg sm:text-xl text-zinc-400 max-w-2xl mx-auto mb-10 leading-relaxed"
        >
          Radarr + Sonarr + Prowlarr in one modern, container-native platform.
          <br className="hidden sm:block" />
          A single Go binary. One database. One UI.
        </motion.p>

        {/* CTA Buttons */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.8 }}
          className="flex flex-col sm:flex-row items-center justify-center gap-4"
        >
          <a
            href="#getting-started"
            className="group flex items-center gap-2 px-8 py-3.5 rounded-xl bg-gradient-to-r from-brand-purple to-brand-blue text-white font-medium shadow-lg shadow-brand-purple/25 hover:shadow-brand-purple/40 transition-all duration-300 hover:scale-105"
          >
            Get Started
            <ArrowRight size={18} className="group-hover:translate-x-1 transition-transform" />
          </a>
          <a
            href="https://github.com/Ebenderooock/loom"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 px-8 py-3.5 rounded-xl border border-zinc-700 text-zinc-300 hover:border-zinc-500 hover:text-white transition-all duration-300 hover:scale-105"
          >
            <GithubIcon />
            View on GitHub
          </a>
        </motion.div>
      </motion.div>

      {/* Scroll indicator */}
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 1.5 }}
        className="absolute bottom-8 left-1/2 -translate-x-1/2"
      >
        <div className="w-6 h-10 rounded-full border-2 border-zinc-600 flex items-start justify-center p-2">
          <motion.div
            animate={{ y: [0, 12, 0] }}
            transition={{ duration: 1.5, repeat: Infinity }}
            className="w-1.5 h-1.5 bg-brand-purple rounded-full"
          />
        </div>
      </motion.div>
    </section>
  )
}

function FilmStrip() {
  return (
    <svg width="60" height="300" viewBox="0 0 60 300" fill="none" className="opacity-30">
      {Array.from({ length: 10 }).map((_, i) => (
        <g key={i}>
          <rect x="0" y={i * 30} width="60" height="28" rx="2" stroke="currentColor" strokeWidth="0.5" fill="none" className="text-zinc-600" />
          <rect x="4" y={i * 30 + 4} width="8" height="20" rx="1" fill="currentColor" className="text-zinc-700" />
          <rect x="48" y={i * 30 + 4} width="8" height="20" rx="1" fill="currentColor" className="text-zinc-700" />
        </g>
      ))}
    </svg>
  )
}
