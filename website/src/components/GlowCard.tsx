import { type ReactNode } from 'react'

interface GlowCardProps {
  children: ReactNode
  className?: string
}

export default function GlowCard({ children, className = '' }: GlowCardProps) {
  return (
    <div className={`relative group ${className}`}>
      {/* Gradient border glow */}
      <div className="absolute -inset-[1px] rounded-2xl bg-gradient-to-r from-brand-purple/20 via-brand-blue/20 to-brand-cyan/20 opacity-0 group-hover:opacity-100 transition-opacity duration-500 blur-sm" />
      <div className="relative glass glass-hover rounded-2xl p-6 h-full transition-all duration-300">
        {children}
      </div>
    </div>
  )
}
