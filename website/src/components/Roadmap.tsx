import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'

const phases = [
  { id: 0, title: 'Foundation', status: 'done', description: 'Project structure, CI/CD, core framework' },
  { id: 1, title: 'Configuration & Storage', status: 'done', description: 'Config system, SQLite, migrations' },
  { id: 2, title: 'Indexer Engine', status: 'done', description: 'Cardigann YAML, Newznab, Torznab, 542 indexers' },
  { id: 3, title: 'Download Clients', status: 'in-progress', description: 'qBittorrent, Transmission, SABnzbd, NZBGet' },
  { id: 4, title: 'Media Management', status: 'planned', description: 'Library scanning, metadata, file management' },
  { id: 5, title: 'Search & Automation', status: 'planned', description: 'RSS, auto-search, quality profiles' },
  { id: 6, title: 'API Compatibility', status: 'planned', description: 'Wire-compatible *arr API endpoints' },
  { id: 7, title: 'Notifications', status: 'planned', description: 'Discord, webhooks, Gotify, Ntfy' },
  { id: 8, title: 'Web UI', status: 'planned', description: 'React 19 frontend, mobile-first' },
  { id: 9, title: 'Import & Migration', status: 'planned', description: 'Import from existing *arr databases' },
  { id: 10, title: 'Plugin SDK', status: 'planned', description: 'gRPC plugin system, out-of-process' },
  { id: 11, title: 'Stable Release', status: 'planned', description: 'v1.0 — production-ready' },
]

function statusIcon(status: string) {
  switch (status) {
    case 'done': return '✅'
    case 'in-progress': return '🚧'
    default: return '⏳'
  }
}

function statusColor(status: string) {
  switch (status) {
    case 'done': return 'border-green-500/50 bg-green-500/5'
    case 'in-progress': return 'border-amber-500/50 bg-amber-500/5'
    default: return 'border-zinc-700/50 bg-zinc-800/20'
  }
}

function dotColor(status: string) {
  switch (status) {
    case 'done': return 'bg-green-400'
    case 'in-progress': return 'bg-amber-400'
    default: return 'bg-zinc-600'
  }
}

export default function Roadmap() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-50px' })

  const completedCount = phases.filter((p) => p.status === 'done').length
  const progress = (completedCount / phases.length) * 100

  return (
    <section id="roadmap" className="relative py-24 sm:py-32">
      <div className="absolute inset-0 bg-gradient-to-b from-transparent via-brand-purple/[0.02] to-transparent" />

      <div className="relative max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-12"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4">
            <span className="text-gradient">Roadmap</span>
          </h2>
          <p className="text-zinc-400 text-lg max-w-2xl mx-auto mb-8">
            Actively building toward a stable release.
          </p>

          {/* Progress bar */}
          <div className="max-w-md mx-auto">
            <div className="flex justify-between text-sm text-zinc-500 mb-2">
              <span>{completedCount} of {phases.length} phases complete</span>
              <span>{Math.round(progress)}%</span>
            </div>
            <div className="h-2 bg-zinc-800 rounded-full overflow-hidden">
              <motion.div
                initial={{ width: 0 }}
                animate={isInView ? { width: `${progress}%` } : {}}
                transition={{ duration: 1.2, delay: 0.5 }}
                className="h-full bg-gradient-to-r from-brand-purple to-brand-blue rounded-full"
              />
            </div>
          </div>
        </motion.div>

        {/* Timeline */}
        <div className="relative">
          {/* Vertical line */}
          <div className="absolute left-4 sm:left-6 top-0 bottom-0 w-px bg-zinc-800" />

          <div className="space-y-4">
            {phases.map((phase, i) => (
              <motion.div
                key={phase.id}
                initial={{ opacity: 0, x: -20 }}
                animate={isInView ? { opacity: 1, x: 0 } : {}}
                transition={{ duration: 0.4, delay: 0.3 + i * 0.06 }}
                className="relative pl-12 sm:pl-16"
              >
                {/* Dot */}
                <div className={`absolute left-2.5 sm:left-4.5 top-4 w-3 h-3 rounded-full ${dotColor(phase.status)} ring-4 ring-zinc-950`} />

                <div className={`p-4 rounded-xl border ${statusColor(phase.status)} transition-colors`}>
                  <div className="flex items-start gap-3">
                    <span className="text-lg">{statusIcon(phase.status)}</span>
                    <div>
                      <h4 className="text-sm font-semibold text-white">
                        Phase {phase.id}: {phase.title}
                      </h4>
                      <p className="text-sm text-zinc-400 mt-0.5">{phase.description}</p>
                    </div>
                  </div>
                </div>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
