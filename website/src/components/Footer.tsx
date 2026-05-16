export default function Footer() {
  return (
    <footer className="border-t border-zinc-800/50 py-12">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex flex-col md:flex-row items-center justify-between gap-6">
          {/* Logo & tagline */}
          <div className="flex items-center gap-3">
            <img src="/loom-logo.png" alt="Loom" width={32} height={32} className="w-8 h-8" />
            <div>
              <span className="font-bold text-gradient">Loom</span>
              <p className="text-xs text-zinc-500">Unified Media Automation</p>
            </div>
          </div>

          {/* Links */}
          <div className="flex items-center gap-6 text-sm text-zinc-500">
            <a href="https://github.com/Ebenderooock/loom" target="_blank" rel="noopener noreferrer" className="hover:text-white transition-colors focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950 rounded">
              GitHub
            </a>
            <a href="https://github.com/Ebenderooock/loom/blob/main/LICENSE" target="_blank" rel="noopener noreferrer" className="hover:text-white transition-colors focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950 rounded">
              AGPL-3.0
            </a>
            <a href="https://github.com/Ebenderooock/loom/discussions" target="_blank" rel="noopener noreferrer" className="hover:text-white transition-colors focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950 rounded">
              Discussions
            </a>
          </div>
        </div>

        <div className="mt-8 pt-6 border-t border-zinc-800/50 text-center">
          <p className="text-xs text-zinc-600">
            Not a fork — a clean-room reimplementation. Built with ❤️ in Go.
          </p>
        </div>
      </div>
    </footer>
  )
}
