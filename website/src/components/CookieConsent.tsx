import { useEffect, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { Cookie, X } from 'lucide-react'

const STORAGE_KEY = 'loom-cookie-consent'

type Choice = 'accepted' | 'declined'

// Cloudflare Zaraz consent API (present only when Zaraz is enabled on the zone).
type ZarazConsent = {
  setAll?: (granted: boolean) => void
  sendQueuedEvents?: () => void
}

declare global {
  interface Window {
    zaraz?: { consent?: ZarazConsent }
  }
}

function applyZarazConsent(granted: boolean) {
  const consent = window.zaraz?.consent
  if (!consent) return
  try {
    consent.setAll?.(granted)
    if (granted) consent.sendQueuedEvents?.()
  } catch {
    /* Zaraz not fully initialised — safe to ignore */
  }
}

function readStoredChoice(): Choice | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'accepted' || stored === 'declined') return stored
  } catch {
    /* storage blocked (private mode) — treat as no choice yet */
  }
  return null
}

export default function CookieConsent() {
  const [choice, setChoice] = useState<Choice | null>(readStoredChoice)

  useEffect(() => {
    if (choice) applyZarazConsent(choice === 'accepted')
  }, [choice])

  const decide = (next: Choice) => {
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      /* ignore persistence failure */
    }
    setChoice(next)
  }

  const visible = choice === null

  return (
    <AnimatePresence>
      {visible && (
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 24 }}
          transition={{ duration: 0.4, ease: [0.16, 1, 0.3, 1] }}
          role="dialog"
          aria-modal="false"
          aria-labelledby="cookie-consent-title"
          aria-describedby="cookie-consent-desc"
          className="fixed bottom-4 left-4 right-4 sm:left-auto sm:right-6 sm:bottom-6 z-[120] sm:max-w-sm"
        >
          <div className="glass glow-purple rounded-2xl border border-brand-purple/20 p-5 shadow-2xl">
            <div className="flex items-start gap-3">
              <div className="feature-icon-wrap mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-brand-purple/10 border border-brand-purple/20">
                <Cookie size={18} className="text-brand-purple" aria-hidden="true" />
              </div>
              <div className="min-w-0 flex-1">
                <h2
                  id="cookie-consent-title"
                  className="text-sm font-semibold text-white"
                >
                  We value your privacy
                </h2>
                <p
                  id="cookie-consent-desc"
                  className="mt-1 text-xs leading-relaxed text-zinc-400"
                >
                  Loom uses privacy-friendly analytics to understand how the site
                  is used. You can accept or decline — your choice is remembered.
                </p>
              </div>
              <button
                onClick={() => decide('declined')}
                aria-label="Decline and dismiss"
                className="shrink-0 text-zinc-500 hover:text-white transition-colors focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950 rounded"
              >
                <X size={16} aria-hidden="true" />
              </button>
            </div>

            <div className="mt-4 flex items-center gap-2.5">
              <button
                onClick={() => decide('accepted')}
                className="flex-1 rounded-lg bg-brand-purple px-4 py-2 text-sm font-medium text-white transition-all duration-200 hover:bg-brand-purple/90 hover:shadow-[0_0_24px_rgba(139,92,246,0.4)] focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950"
              >
                Accept
              </button>
              <button
                onClick={() => decide('declined')}
                className="flex-1 rounded-lg border border-zinc-700 bg-zinc-800/40 px-4 py-2 text-sm font-medium text-zinc-300 transition-all duration-200 hover:bg-zinc-800 hover:text-white focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950"
              >
                Decline
              </button>
            </div>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
