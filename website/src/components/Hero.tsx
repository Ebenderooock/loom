import { motion, useScroll, useTransform, AnimatePresence } from 'framer-motion'
import { useRef, useState, useEffect } from 'react'
import { ArrowRight, Play, Film, Tv, Download } from 'lucide-react'
import { GithubIcon } from './icons'

const rotatingWords = ['Movies', 'TV Shows', 'Indexers', 'Torrents', 'Libraries']

export default function Hero() {
  const ref = useRef(null)
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ['start start', 'end start'],
  })
  const y = useTransform(scrollYProgress, [0, 1], [0, 200])
  const opacity = useTransform(scrollYProgress, [0, 0.8], [1, 0])

  const [wordIndex, setWordIndex] = useState(0)
  useEffect(() => {
    const interval = setInterval(() => {
      setWordIndex((prev) => (prev + 1) % rotatingWords.length)
    }, 2500)
    return () => clearInterval(interval)
  }, [])

  return (
    <section ref={ref} className="relative min-h-screen flex items-center justify-center overflow-hidden pt-16">
      {/* Animated background */}
      <div className="absolute inset-0">
        {/* Gradient orbs */}
        <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-brand-purple/20 rounded-full blur-[128px] animate-pulse-glow" />
        <div className="absolute bottom-1/4 right-1/4 w-96 h-96 bg-brand-blue/15 rounded-full blur-[128px] animate-pulse-glow" style={{ animationDelay: '2s' }} />
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-brand-cyan/5 rounded-full blur-[128px] animate-float-slow" />

        {/* Film strip decorations */}
        <div className="absolute top-20 left-10 opacity-5" aria-hidden="true">
          <FilmStrip />
        </div>
        <div className="absolute bottom-20 right-10 opacity-5" aria-hidden="true">
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

        {/* Floating media cards */}
        <div className="absolute top-[15%] left-[8%] opacity-[0.07] animate-float-drift-1">
          <div className="w-28 h-40 rounded-lg bg-gradient-to-br from-brand-purple/40 to-brand-blue/30 border border-white/10 flex items-center justify-center">
            <Play size={24} className="text-white/60" />
          </div>
        </div>
        <div className="absolute top-[25%] right-[10%] opacity-[0.06] animate-float-drift-2" style={{ animationDelay: '2s' }}>
          <div className="w-24 h-32 rounded-lg bg-gradient-to-br from-brand-blue/30 to-brand-cyan/30 border border-white/10 p-3">
            <div className="w-full h-3 bg-white/20 rounded mb-2" />
            <div className="w-3/4 h-2 bg-white/10 rounded mb-1" />
            <div className="w-1/2 h-2 bg-white/10 rounded" />
            <Film size={16} className="text-white/40 mt-3" />
          </div>
        </div>
        <div className="absolute bottom-[30%] left-[12%] opacity-[0.05] animate-float-drift-3" style={{ animationDelay: '4s' }}>
          <div className="w-32 h-20 rounded-lg bg-gradient-to-br from-brand-pink/20 to-brand-purple/30 border border-white/10 p-3 flex flex-col justify-between">
            <Tv size={14} className="text-white/40" />
            <div className="w-full h-1.5 bg-white/10 rounded-full overflow-hidden">
              <div className="w-2/3 h-full bg-brand-purple/50 rounded-full" />
            </div>
          </div>
        </div>
        <div className="absolute bottom-[20%] right-[8%] opacity-[0.06] animate-float-drift-1" style={{ animationDelay: '3s' }}>
          <div className="w-24 h-24 rounded-lg bg-gradient-to-br from-brand-cyan/20 to-brand-blue/30 border border-white/10 flex items-center justify-center">
            <Download size={20} className="text-white/40" />
          </div>
        </div>
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
            <img src="/loom-logo.png" alt="Loom" width={128} height={128} className="relative w-24 h-24 sm:w-32 sm:h-32 mx-auto" />
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

        {/* Subtitle with rotating words */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.6 }}
          className="text-lg sm:text-xl text-zinc-400 max-w-2xl mx-auto mb-10 leading-relaxed"
        >
          <span className="inline-flex items-baseline justify-center flex-wrap">
            <AnimatePresence mode="wait">
              <motion.span
                key={rotatingWords[wordIndex]}
                initial={{ opacity: 0, y: 20, filter: 'blur(4px)' }}
                animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
                exit={{ opacity: 0, y: -20, filter: 'blur(4px)' }}
                transition={{ duration: 0.4 }}
                className="inline-block text-brand-purple font-semibold min-w-[120px] text-right"
              >
                {rotatingWords[wordIndex]}
              </motion.span>
            </AnimatePresence>
            <span>&nbsp;— one modern, container-native platform.</span>
          </span>
          <br className="hidden sm:block" />
          <span>A single Go binary. One database. One UI.</span>
        </motion.div>

        {/* CTA Buttons */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.8 }}
          className="flex flex-col sm:flex-row items-center justify-center gap-4"
        >
          <a
            href="#getting-started"
            className="group flex items-center gap-2 px-8 py-3.5 rounded-xl bg-gradient-to-r from-brand-purple to-brand-blue text-white font-medium shadow-lg shadow-brand-purple/25 hover:shadow-brand-purple/40 transition-shadow duration-300 hover:scale-105 focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950"
          >
            Get Started
            <ArrowRight size={18} className="group-hover:translate-x-1 transition-transform" />
          </a>
          <a
            href="https://github.com/Ebenderooock/loom"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 px-8 py-3.5 rounded-xl border border-zinc-700 text-zinc-300 hover:border-zinc-500 hover:text-white transition-all duration-300 hover:scale-105 focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950"
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
    <svg width="60" height="300" viewBox="0 0 60 300" fill="none" className="opacity-30" aria-hidden="true">
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
