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

export default function App() {
  return (
    <div className="min-h-screen bg-zinc-950 text-white overflow-x-hidden">
      <Navbar />
      <Hero />
      <WhatIsLoom />
      <Features />
      <WhyLoom />
      <Architecture />
      <GettingStarted />
      <Roadmap />
      <Community />
      <Footer />
    </div>
  )
}
