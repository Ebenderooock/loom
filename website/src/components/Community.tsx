import { motion, useInView } from 'framer-motion'
import { useRef, useCallback } from 'react'
import { MessageCircle, BookOpen, Star } from 'lucide-react'
import { GithubIcon } from './icons'

const links = [
  {
    icon: GithubIcon,
    title: 'GitHub',
    description: 'Star the repo, file issues, contribute code',
    href: 'https://github.com/Ebenderooock/loom',
    cta: 'View Repository',
  },
  {
    icon: MessageCircle,
    title: 'Discord',
    description: 'Join the community, get help, share feedback',
    href: 'https://github.com/Ebenderooock/loom/discussions',
    cta: 'Join Discussions',
  },
  {
    icon: BookOpen,
    title: 'Contributing',
    description: 'Read the guide and submit your first PR',
    href: 'https://github.com/Ebenderooock/loom/blob/main/CONTRIBUTING.md',
    cta: 'Read Guide',
  },
]

export default function Community() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-50px' })

  const handleTiltMove = useCallback((e: React.MouseEvent<HTMLAnchorElement>) => {
    const el = e.currentTarget
    const rect = el.getBoundingClientRect()
    const x = (e.clientX - rect.left) / rect.width - 0.5
    const y = (e.clientY - rect.top) / rect.height - 0.5
    el.style.transform = `perspective(1000px) rotateY(${x * 8}deg) rotateX(${-y * 8}deg) scale(1.02)`
  }, [])

  const handleTiltLeave = useCallback((e: React.MouseEvent<HTMLAnchorElement>) => {
    e.currentTarget.style.transform = 'perspective(1000px) rotateY(0deg) rotateX(0deg) scale(1)'
  }, [])

  return (
    <section className="relative py-24 sm:py-32">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-16"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4">
            Join the <span className="text-gradient">Community</span>
          </h2>
          <p className="text-zinc-400 text-lg max-w-2xl mx-auto">
            Loom is open source and community-driven. Every contribution matters.
          </p>
        </motion.div>

        <div className="grid sm:grid-cols-3 gap-6 mb-12">
          {links.map((link, i) => (
            <motion.a
              key={link.title}
              href={link.href}
              target="_blank"
              rel="noopener noreferrer"
              initial={{ opacity: 0, y: 30 }}
              animate={isInView ? { opacity: 1, y: 0 } : {}}
              transition={{ duration: 0.5, delay: 0.2 + i * 0.1 }}
              onMouseMove={handleTiltMove}
              onMouseLeave={handleTiltLeave}
              className="group glass glass-hover rounded-2xl p-6 text-center tilt-card focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950"
            >
              <link.icon size={28} className="mx-auto mb-4 text-brand-purple group-hover:text-brand-blue transition-colors" />
              <h3 className="text-lg font-semibold text-white mb-2">{link.title}</h3>
              <p className="text-sm text-zinc-400 mb-4">{link.description}</p>
              <span className="text-sm text-brand-purple font-medium group-hover:text-brand-blue transition-colors">
                {link.cta} →
              </span>
            </motion.a>
          ))}
        </div>

        {/* Star CTA */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.6, delay: 0.6 }}
          className="text-center"
        >
          <a
            href="https://github.com/Ebenderooock/loom"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-gradient-to-r from-brand-purple/10 to-brand-blue/10 border border-brand-purple/30 text-white hover:border-brand-purple/60 transition-all duration-300 hover:scale-105 focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950"
          >
            <Star size={18} className="text-yellow-400" />
            <span>Star on GitHub</span>
          </a>
        </motion.div>
      </div>
    </section>
  )
}
