import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'

export default function Architecture() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-50px' })

  return (
    <section className="relative py-24 sm:py-32 overflow-hidden">
      {/* Background */}
      <div className="absolute inset-0 bg-gradient-to-b from-transparent via-brand-blue/[0.02] to-transparent" />

      <div className="relative max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-16"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4">
            <span className="text-gradient">Architecture</span> Overview
          </h2>
          <p className="text-zinc-400 text-lg max-w-2xl mx-auto">
            Everything connects through one unified system.
          </p>
        </motion.div>

        {/* Architecture diagram */}
        <motion.div
          initial={{ opacity: 0, scale: 0.95 }}
          animate={isInView ? { opacity: 1, scale: 1 } : {}}
          transition={{ duration: 0.8, delay: 0.2 }}
          className="relative"
        >
          <div className="glass rounded-2xl p-6 sm:p-10">
            <div className="grid grid-cols-1 lg:grid-cols-5 gap-6 items-center">
              {/* Sources */}
              <div className="space-y-3">
                <h4 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4 text-center">Sources</h4>
                {['Newznab', 'Torznab', 'Cardigann'].map((s) => (
                  <div key={s} className="px-3 py-2 rounded-lg bg-zinc-800/50 border border-zinc-700/50 text-sm text-zinc-300 text-center">
                    {s}
                  </div>
                ))}
              </div>

              {/* Arrow */}
              <div className="hidden lg:flex items-center justify-center">
                <svg width="60" height="20" className="text-brand-purple/50" aria-hidden="true">
                  <line x1="0" y1="10" x2="50" y2="10" stroke="currentColor" strokeWidth="2" strokeDasharray="6 4" className="animate-dash-flow" />
                  <polygon points="50,5 60,10 50,15" fill="currentColor" />
                </svg>
              </div>

              {/* Core */}
              <div className="relative">
                <div className="absolute -inset-4 bg-gradient-to-r from-brand-purple/10 via-brand-blue/10 to-brand-cyan/10 rounded-2xl blur-xl" />
                <div className="relative p-6 rounded-2xl border-2 border-brand-purple/30 bg-zinc-900/80 text-center animate-core-pulse">
                  <img src="/loom-logo.png" alt="Loom" className="h-10 w-auto mx-auto mb-3" />
                  <h3 className="text-lg font-bold text-gradient mb-2">Loom Core</h3>
                  <div className="space-y-1.5 text-xs text-zinc-400">
                    <div className="px-2 py-1 rounded bg-zinc-800/50">Indexer Manager</div>
                    <div className="px-2 py-1 rounded bg-zinc-800/50">Media Library</div>
                    <div className="px-2 py-1 rounded bg-zinc-800/50">Download Orchestrator</div>
                    <div className="px-2 py-1 rounded bg-zinc-800/50">Metadata Engine</div>
                  </div>
                </div>
              </div>

              {/* Arrow */}
              <div className="hidden lg:flex items-center justify-center">
                <svg width="60" height="20" className="text-brand-blue/50" aria-hidden="true">
                  <line x1="0" y1="10" x2="50" y2="10" stroke="currentColor" strokeWidth="2" strokeDasharray="6 4" className="animate-dash-flow" />
                  <polygon points="50,5 60,10 50,15" fill="currentColor" />
                </svg>
              </div>

              {/* Outputs */}
              <div className="space-y-3">
                <h4 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4 text-center">Integrations</h4>
                {['Plex / Jellyfin', 'Download Clients', 'Notifications'].map((s) => (
                  <div key={s} className="px-3 py-2 rounded-lg bg-zinc-800/50 border border-zinc-700/50 text-sm text-zinc-300 text-center">
                    {s}
                  </div>
                ))}
              </div>
            </div>

            {/* Bottom: Database */}
            <div className="mt-8 pt-6 border-t border-zinc-800/50 flex justify-center">
              <div className="flex items-center gap-3 px-4 py-2 rounded-lg bg-zinc-800/30 border border-zinc-700/30">
                <div className="w-2 h-2 rounded-full bg-green-400" />
                <span className="text-sm text-zinc-400">SQLite / PostgreSQL — Single Database</span>
              </div>
            </div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
