import { motion, useInView } from 'framer-motion'
import { useRef } from 'react'
import CodeBlock from './CodeBlock'

const installMethods = [
  {
    title: 'Docker',
    code: `docker run -d \\
  --name loom \\
  -p 8989:8989 \\
  -v /path/to/config:/config \\
  -v /path/to/media:/media \\
  ghcr.io/ebenderooock/loom:latest`,
  },
  {
    title: 'Docker Compose',
    code: `services:
  loom:
    image: ghcr.io/ebenderooock/loom:latest
    container_name: loom
    ports:
      - "8989:8989"
    volumes:
      - ./config:/config
      - /media:/media
    restart: unless-stopped`,
  },
  {
    title: 'From Source',
    code: `git clone https://github.com/Ebenderooock/loom.git
cd loom
make build
LOOM_CONFIG_DIR=./run LOOM_DATA_DIR=./run \\
  LOOM_STORAGE_SQLITE_PATH=./run/loom.db \\
  ./dist/loom serve`,
  },
]

export default function GettingStarted() {
  const ref = useRef(null)
  const isInView = useInView(ref, { once: true, margin: '-50px' })

  return (
    <section id="getting-started" className="relative py-24 sm:py-32">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          ref={ref}
          initial={{ opacity: 0, y: 30 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.7 }}
          className="text-center mb-16"
        >
          <h2 className="text-3xl sm:text-4xl font-bold mb-4">
            Getting <span className="text-gradient">Started</span>
          </h2>
          <p className="text-zinc-400 text-lg max-w-2xl mx-auto">
            Up and running in under a minute. Choose your preferred method.
          </p>
        </motion.div>

        <div className="space-y-6">
          {installMethods.map((method, i) => (
            <motion.div
              key={method.title}
              initial={{ opacity: 0, y: 30 }}
              animate={isInView ? { opacity: 1, y: 0 } : {}}
              transition={{ duration: 0.5, delay: 0.2 + i * 0.15 }}
            >
              <CodeBlock code={method.code} title={method.title} />
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
