import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'
import { Box, Database, Layout } from 'lucide-react'
import GlowCard from './GlowCard'

const highlights = [
  {
    icon: Box,
    title: 'One Binary',
    description: 'A single ~60MB Go binary. Deploy once, manage movies, TV shows, and indexers from one place.',
    gradient: 'from-brand-purple to-brand-blue',
  },
  {
    icon: Database,
    title: 'One Database',
    description: 'SQLite by default, Postgres-ready. No more juggling three separate database files.',
    gradient: 'from-brand-blue to-brand-cyan',
  },
  {
    icon: Layout,
    title: 'One UI',
    description: 'A unified React 18 interface for movies, TV, and indexers. Mobile-first, modern design.',
    gradient: 'from-brand-cyan to-brand-purple',
  },
]

export default function WhatIsLoom() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-100px' })

  return (
    <section className="relative py-24 sm:py-32">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-16"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4" style={{ textWrap: 'balance' }}>
            What is <span className="text-gradient">Loom</span>?
          </h2>
          <p className="text-zinc-400 text-lg max-w-3xl mx-auto leading-relaxed">
            Loom is a from-scratch, clean-room media automation platform. Not a fork —
            a complete rethink built with modern tooling, designed for
            containers, and obsessed with simplicity.
          </p>
        </motion.div>

        <div className="grid md:grid-cols-3 gap-6">
          {highlights.map((item, i) => (
            <motion.div
              key={item.title}
              initial={{ opacity: 0, y: 40 }}
              animate={isInView ? { opacity: 1, y: 0 } : {}}
              transition={{ duration: 0.6, delay: 0.2 + i * 0.15 }}
            >
              <GlowCard className="h-full">
                <div className={`inline-flex p-3 rounded-xl bg-gradient-to-br ${item.gradient} mb-4`}>
                  <item.icon size={24} className="text-white" />
                </div>
                <h3 className="text-xl font-semibold text-white mb-2">{item.title}</h3>
                <p className="text-zinc-400 leading-relaxed">{item.description}</p>
              </GlowCard>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
