import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'

const phases = [
  { id: 0, title: 'Foundation', status: 'done', description: 'Project structure, CI/CD, Makefile, core framework, SQLite with migrations' },
  { id: 1, title: 'Indexer Engine', status: 'done', description: 'Cardigann YAML engine, Newznab & Torznab support, 500+ indexers built in' },
  { id: 2, title: 'Download Clients', status: 'done', description: 'qBittorrent, Transmission, Deluge, rTorrent, SABnzbd, NZBGet integration' },
  { id: 3, title: 'Built-in Torrent Client', status: 'done', description: 'Zero-dependency BitTorrent client baked into the binary — no external client needed' },
  { id: 4, title: 'Workflow Engine', status: 'done', description: 'Full state machine: search → grab → download → post-download → import → complete' },
  { id: 5, title: 'Smart Retry & Recovery', status: 'done', description: 'Intelligent retry from optimal point, boot reconciliation for interrupted imports' },
  { id: 6, title: 'Media Management', status: 'done', description: 'Movies & TV shows library, metadata from TMDB, file renaming, collision detection' },
  { id: 7, title: 'Web UI', status: 'done', description: 'React 18 + TypeScript frontend — library, search, downloads, workflows, settings' },
  { id: 8, title: 'API Compatibility', status: 'done', description: 'Wire-compatible *arr API endpoints for Overseerr, Jellyseerr, Bazarr, and more' },
  { id: 9, title: 'Observability', status: 'done', description: 'OpenTelemetry, Prometheus /metrics, structured JSON logs, pprof profiling' },
  { id: 10, title: 'Notifications', status: 'done', description: 'Discord, Webhook, Gotify, Ntfy — grabs, downloads, imports, and health alerts' },
  { id: 11, title: 'Import Lists', status: 'done', description: 'Automatic library sync from Trakt, IMDB, Plex watchlists, and more' },
  { id: 12, title: 'TV Show Automation', status: 'done', description: 'Season pack handling, episode monitoring, automatic searches for new episodes' },
  { id: 13, title: 'Import & Migration', status: 'done', description: 'One-click import from existing Radarr, Sonarr, and Prowlarr databases' },
  { id: 14, title: 'Quality Profiles', status: 'done', description: 'Custom quality rankings, scoring, custom formats, and upgrade-until thresholds' },
  { id: 15, title: 'Media Requests', status: 'done', description: 'User request portal, Discord & Telegram bots, approval workflows & per-user quotas — Overseerr built in' },
  { id: 16, title: 'Multi-User & Invites', status: 'done', description: 'Admin/user roles, self-service invite links, and a request/approval flow for friends and family' },
  { id: 17, title: 'Media Server Analytics', status: 'done', description: 'Play session monitoring, watch history, bandwidth & transcode stats, playback alerts — Tautulli built in' },
  { id: 18, title: 'Plugin & Script Engine', status: 'done', description: 'JavaScript plugins with an in-browser Monaco editor — hook into grabs, imports, downloads & notifications' },
  { id: 19, title: 'Usenet / NZB Support', status: 'planned', description: 'Built-in NZB downloading and Usenet indexer integration — no external NZB client needed' },
  { id: 20, title: 'Library Maintenance', status: 'planned', description: 'Duplicate detection, orphan cleanup, quality upgrade pruning — Maintainerr built in' },
  { id: 21, title: 'Script Marketplace', status: 'planned', description: 'Browse, install, and share plugins built by the community — turning users into contributors who make Loom better for everyone' },
  { id: 22, title: 'Stable Release', status: 'planned', description: 'v1.0 — production-ready, battle-tested, fully documented' },
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
          {/* Vertical line - base */}
          <div className="absolute left-4 sm:left-6 top-0 bottom-0 w-px bg-zinc-800" />
          {/* Animated progress overlay */}
          <motion.div
            className="absolute left-4 sm:left-6 top-0 w-px"
            style={{ background: `linear-gradient(to bottom, #22c55e 0%, #22c55e ${progress * 0.9}%, #f59e0b ${progress * 0.95}%, #3f3f46 ${progress}%)` }}
            initial={{ height: 0 }}
            animate={isInView ? { height: '100%' } : {}}
            transition={{ duration: 2, delay: 0.5, ease: 'easeOut' }}
          />

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
