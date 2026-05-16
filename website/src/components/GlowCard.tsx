import { type ReactNode, useRef, useCallback } from 'react'

interface GlowCardProps {
  children: ReactNode
  className?: string
}

export default function GlowCard({ children, className = '' }: GlowCardProps) {
  const cardRef = useRef<HTMLDivElement>(null)

  const handleMouseMove = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const el = cardRef.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    const x = ((e.clientX - rect.left) / rect.width) * 100
    const y = ((e.clientY - rect.top) / rect.height) * 100
    el.style.setProperty('--glow-x', `${x}%`)
    el.style.setProperty('--glow-y', `${y}%`)
  }, [])

  return (
    <div className={`relative group ${className}`}>
      {/* Gradient border glow */}
      <div className="absolute -inset-[1px] rounded-2xl bg-gradient-to-r from-brand-purple/20 via-brand-blue/20 to-brand-cyan/20 opacity-0 group-hover:opacity-100 transition-opacity duration-500 blur-sm" />
      <div
        ref={cardRef}
        onMouseMove={handleMouseMove}
        className="relative glass glass-hover rounded-2xl p-6 h-full transition-all duration-300 glow-card-track overflow-hidden"
      >
        {children}
      </div>
    </div>
  )
}
