import { motion, useInView } from 'framer-motion'
import { useRef, useState, useEffect } from 'react'
import { Check, X, Minus } from 'lucide-react'

function AnimatedNumber({ value, isInView }: { value: number; isInView: boolean }) {
  const [display, setDisplay] = useState(0)
  useEffect(() => {
    if (!isInView) return
    let start = 0
    const duration = 1200
    const startTime = performance.now()
    const animate = (now: number) => {
      const elapsed = now - startTime
      const progress = Math.min(elapsed / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      start = Math.round(eased * value)
      setDisplay(start)
      if (progress < 1) requestAnimationFrame(animate)
    }
    requestAnimationFrame(animate)
  }, [isInView, value])
  return <>{display}</>
}

const comparisons = [
  { category: 'Apps to deploy', arr: '7+ separate apps', loom: '1 binary', loomBetter: true },
  { category: 'Movies (Radarr)', arr: 'Radarr — dedicated .NET app', loom: 'Built in', loomBetter: true },
  { category: 'TV Shows (Sonarr)', arr: 'Sonarr — dedicated .NET app', loom: 'Built in', loomBetter: true },
  { category: 'Indexers (Prowlarr)', arr: 'Prowlarr — dedicated .NET app', loom: 'Built in — 500+ indexers', loomBetter: true },
  { category: 'Torrent Client', arr: 'qBittorrent / Transmission / etc.', loom: 'Built in — zero dependencies', loomBetter: true },
  { category: 'Requests (Overseerr)', arr: 'Overseerr / Ombi — separate app', loom: 'Built in — portal + bots', loomBetter: true },
  { category: 'Analytics (Tautulli)', arr: 'Tautulli — separate app', loom: 'Built in', loomBetter: true },
  { category: 'Custom Scripts', arr: 'Per-app shell hooks', loom: 'Built in — JS plugin engine + editor', loomBetter: true },
  { category: 'Library Cleanup (Maintainerr)', arr: 'Maintainerr — separate app', loom: 'Planned — built in', loomBetter: null },
  { category: 'Databases', arr: '3+ separate SQLite files', loom: '1 (SQLite or Postgres)', loomBetter: true },
  { category: 'Runtime', arr: '.NET + Python + Node.js', loom: 'Single Go binary', loomBetter: true },
  { category: 'Image size', arr: '~200MB+ each', loom: '~60MB distroless', loomBetter: true },
  { category: 'UI', arr: '7+ separate interfaces', loom: 'Unified React 19 + mobile-first', loomBetter: true },
  { category: 'Observability', arr: 'Limited logs', loom: 'OpenTelemetry + Prometheus + pprof', loomBetter: true },
  { category: 'Migration', arr: 'N/A', loom: 'Built-in Radarr/Sonarr/Prowlarr importer', loomBetter: true },
]

export default function WhyLoom() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-50px' })

  return (
    <section id="why-loom" className="relative py-24 sm:py-32">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-16"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4">
            Why <span className="text-gradient">Loom</span>?
          </h2>
          <p className="text-zinc-400 text-lg max-w-2xl mx-auto">
            See how Loom compares to running the traditional *arr stack.
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 40 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7, delay: 0.2 }}
          className="glass rounded-2xl overflow-hidden"
        >
          <table className="w-full">
            <thead>
              <tr className="border-b border-zinc-800 bg-zinc-900/30">
                <th className="text-left text-sm font-medium text-zinc-500 uppercase tracking-wider p-4 sm:p-6">Category</th>
                <th className="text-center text-sm font-medium text-zinc-500 uppercase tracking-wider p-4 sm:p-6">*arr Stack</th>
                <th className="text-center text-sm font-medium text-brand-purple uppercase tracking-wider p-4 sm:p-6">Loom</th>
              </tr>
            </thead>
            <tbody>
              {comparisons.map((row, i) => (
                <motion.tr
                  key={row.category}
                  initial={{ opacity: 0, x: -20 }}
                  animate={isInView ? { opacity: 1, x: 0 } : {}}
                  transition={{ duration: 0.4, delay: 0.3 + i * 0.06 }}
                  className={`border-b border-zinc-800/50 last:border-0 hover:bg-zinc-900/30 transition-colors ${isInView ? 'animate-row-highlight' : ''}`}
                  style={{ animationDelay: `${0.4 + i * 0.08}s` }}
                >
                  <td className="text-sm text-zinc-300 font-medium p-4 sm:px-6">{row.category}</td>
                  <td className="text-sm text-zinc-500 text-center p-4 sm:px-6">
                    <span className="inline-flex items-center justify-center gap-1.5">
                      {row.loomBetter === true && <X size={14} className="text-red-400 shrink-0" aria-hidden="true" />}
                      {row.loomBetter === null && <Minus size={14} className="text-zinc-600 shrink-0" aria-hidden="true" />}
                      <span className="hidden sm:inline">
                        {i === 0 ? <><AnimatedNumber value={7} isInView={isInView} />+ separate apps</> : row.arr}
                      </span>
                      <span className="sm:hidden text-xs">{row.arr.split(' ')[0]}</span>
                    </span>
                  </td>
                  <td className="text-sm text-center p-4 sm:px-6">
                    <span className="inline-flex items-center justify-center gap-1.5">
                      {row.loomBetter === true && <Check size={14} className="text-green-400 shrink-0" aria-hidden="true" />}
                      {row.loomBetter === null && <Minus size={14} className="text-zinc-600 shrink-0" aria-hidden="true" />}
                      <span className={`${row.loomBetter === true ? 'text-green-300' : 'text-zinc-400'}`}>
                        <span className="hidden sm:inline">
                          {i === 0 ? <><AnimatedNumber value={1} isInView={isInView} /> binary</> : row.loom}
                        </span>
                        <span className="sm:hidden text-xs">{row.loom.split(' ')[0]}</span>
                      </span>
                    </span>
                  </td>
                </motion.tr>
              ))}
            </tbody>
          </table>
        </motion.div>
      </div>
    </section>
  )
}
