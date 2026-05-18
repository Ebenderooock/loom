import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'
import {
  Layers,
  Search,
  Download,
  Monitor,
  Activity,
  Cable,
  ShieldCheck,
  Bell,
  HardDrive,
  RotateCcw,
} from 'lucide-react'
import GlowCard from './GlowCard'

const features = [
  {
    icon: Layers,
    title: 'Unified Stack',
    description: 'Movies, TV shows, and indexers managed in one application. No more context switching between apps.',
  },
  {
    icon: Search,
    title: '542 Indexers',
    description: 'Cardigann YAML engine with Newznab & Torznab support. All your sources, built in from day one.',
  },
  {
    icon: HardDrive,
    title: 'Built-in Torrent Client',
    description: 'Zero-dependency torrenting baked right in. No external download client needed — just Loom and your indexers.',
  },
  {
    icon: Download,
    title: 'External Clients Too',
    description: 'Prefer your own client? qBittorrent, Transmission, Deluge, rTorrent, SABnzbd, NZBGet all supported.',
  },
  {
    icon: RotateCcw,
    title: 'Smart Workflows',
    description: 'Intelligent retry picks up where it left off. Boot reconciliation recovers in-flight imports after restarts.',
  },
  {
    icon: Monitor,
    title: 'Modern UI',
    description: 'React 19 + TypeScript. Mobile-first, responsive, fast. A unified interface for your entire library.',
  },
  {
    icon: Activity,
    title: 'Observable',
    description: 'OpenTelemetry, Prometheus /metrics, structured JSON logs, and pprof. Debug anything in production.',
  },
  {
    icon: Cable,
    title: 'Wire-Compatible',
    description: 'Overseerr, Jellyseerr, Bazarr, Notifiarr, Tautulli, Plex, Jellyfin, Emby all keep working.',
  },
  {
    icon: ShieldCheck,
    title: 'Download Safety',
    description: 'Post-download settling, seed ratio enforcement, file renaming, and collision detection before import.',
  },
  {
    icon: Bell,
    title: 'Notifications',
    description: 'Discord, Webhook, Gotify, Ntfy. Get notified about grabs, downloads, and health checks.',
  },
]

export default function Features() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-50px' })

  return (
    <section id="features" className="relative py-24 sm:py-32">
      {/* Background accent */}
      <div className="absolute inset-0 bg-gradient-to-b from-transparent via-brand-purple/[0.02] to-transparent" />

      <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-16"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4">
            Built for <span className="text-gradient">Power Users</span>
          </h2>
          <p className="text-zinc-400 text-lg max-w-2xl mx-auto">
            Everything you need for media automation, unified and enhanced.
          </p>
        </motion.div>

        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-5">
          {features.map((feature, i) => {
            const row = Math.floor(i / 4)
            const col = i % 4
            const staggerDelay = 0.1 + (row + col) * 0.12
            return (
              <motion.div
                key={feature.title}
                initial={{ opacity: 0, y: 30 }}
                animate={isInView ? { opacity: 1, y: 0 } : {}}
                transition={{ duration: 0.5, delay: staggerDelay }}
              >
                <GlowCard className="h-full">
                  <div className="feature-icon-wrap inline-block mb-3">
                    <feature.icon size={22} className="text-brand-purple group-hover:text-transparent group-hover:bg-gradient-to-r group-hover:from-brand-purple group-hover:to-brand-blue group-hover:bg-clip-text transition-colors duration-300" />
                  </div>
                  <h3 className="text-base font-semibold text-white mb-2">{feature.title}</h3>
                  <p className="text-sm text-zinc-400 leading-relaxed">{feature.description}</p>
                </GlowCard>
              </motion.div>
            )
          })}
        </div>
      </div>
    </section>
  )
}
