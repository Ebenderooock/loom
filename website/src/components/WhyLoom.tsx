import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'
import { Check, X, Minus } from 'lucide-react'

const comparisons = [
  { category: 'Apps to deploy', arr: '3 (Radarr + Sonarr + Prowlarr)', loom: '1', loomBetter: true },
  { category: 'Databases', arr: '3 separate SQLite files', loom: '1 (SQLite or Postgres)', loomBetter: true },
  { category: 'Runtime', arr: '.NET (heavy)', loom: 'Go (lightweight)', loomBetter: true },
  { category: 'Image size', arr: '~200MB+ each', loom: '~30MB distroless', loomBetter: true },
  { category: 'Observability', arr: 'Limited logs', loom: 'OpenTelemetry + Prometheus + pprof', loomBetter: true },
  { category: 'UI', arr: '3 separate interfaces', loom: 'Unified React 19 + mobile-first', loomBetter: true },
  { category: 'Migration', arr: 'N/A', loom: 'Built-in *arr DB importer (planned)', loomBetter: null },
  { category: 'Indexer sources', arr: '~700+ (mature)', loom: '542 bundled (growing)', loomBetter: null },
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
          {/* Table header */}
          <div className="grid grid-cols-3 gap-4 p-4 sm:p-6 border-b border-zinc-800 bg-zinc-900/30">
            <div className="text-sm font-medium text-zinc-500 uppercase tracking-wider">Category</div>
            <div className="text-sm font-medium text-zinc-500 uppercase tracking-wider text-center">*arr Stack</div>
            <div className="text-sm font-medium text-brand-purple uppercase tracking-wider text-center">Loom</div>
          </div>

          {/* Rows */}
          {comparisons.map((row, i) => (
            <motion.div
              key={row.category}
              initial={{ opacity: 0, x: -20 }}
              animate={isInView ? { opacity: 1, x: 0 } : {}}
              transition={{ duration: 0.4, delay: 0.3 + i * 0.06 }}
              className="grid grid-cols-3 gap-4 p-4 sm:px-6 border-b border-zinc-800/50 last:border-0 hover:bg-zinc-900/30 transition-colors"
            >
              <div className="text-sm text-zinc-300 font-medium flex items-center">{row.category}</div>
              <div className="text-sm text-zinc-500 text-center flex items-center justify-center gap-1.5">
                {row.loomBetter === true && <X size={14} className="text-red-400 shrink-0" />}
                {row.loomBetter === null && <Minus size={14} className="text-zinc-600 shrink-0" />}
                <span className="hidden sm:inline">{row.arr}</span>
                <span className="sm:hidden text-xs">{row.arr.split(' ')[0]}</span>
              </div>
              <div className="text-sm text-center flex items-center justify-center gap-1.5">
                {row.loomBetter === true && <Check size={14} className="text-green-400 shrink-0" />}
                {row.loomBetter === null && <Minus size={14} className="text-zinc-600 shrink-0" />}
                <span className={`${row.loomBetter === true ? 'text-green-300' : 'text-zinc-400'}`}>
                  <span className="hidden sm:inline">{row.loom}</span>
                  <span className="sm:hidden text-xs">{row.loom.split(' ')[0]}</span>
                </span>
              </div>
            </motion.div>
          ))}
        </motion.div>
      </div>
    </section>
  )
}
