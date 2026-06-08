import Navbar from './components/Navbar'
import Hero from './components/Hero'
import WhatIsLoom from './components/WhatIsLoom'
import Features from './components/Features'
import WhyLoom from './components/WhyLoom'
import Architecture from './components/Architecture'
import GettingStarted from './components/GettingStarted'
import Roadmap from './components/Roadmap'
import Community from './components/Community'
import Footer from './components/Footer'
import CookieConsent from './components/CookieConsent'

export default function App() {
  return (
    <div className="min-h-screen bg-zinc-950 text-white overflow-x-hidden touch-manipulation">
      <a href="#main" className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-[100] focus:px-4 focus:py-2 focus:bg-brand-purple focus:text-white focus:rounded-lg">Skip to Content</a>
      <Navbar />
      <main id="main">
        <Hero />
        <WhatIsLoom />
        <Features />
        <WhyLoom />
        <Architecture />
        <GettingStarted />
        <Roadmap />
        <Community />
        <Footer />
      </main>
      <CookieConsent />
    </div>
  )
}
