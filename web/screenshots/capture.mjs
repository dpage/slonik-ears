// Captures the screenshots used in the documentation and the README, against
// a server seeded by seed.mjs. Viewport heights are chosen so each page fills
// its frame; a taller viewport leaves dead space below the content.
import { chromium } from 'playwright'
import { fileURLToPath } from 'node:url'
import { mkdir } from 'node:fs/promises'

const BASE = process.env.EARS_SHOTS_BASE ?? 'http://127.0.0.1:8131'
const ADMIN = process.env.EARS_SHOTS_ADMIN_TOKEN ?? 'shots-admin'
const OUT = fileURLToPath(new URL('../../docs/img/screens/', import.meta.url))
await mkdir(OUT, { recursive: true })

const browser = await chromium.launch()
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function shot(name, { width, height, path: url, prepare }) {
  const context = await browser.newContext({
    viewport: { width, height },
    deviceScaleFactor: 2,
    colorScheme: 'dark',
    isMobile: width < 600,
    hasTouch: width < 600,
  })
  const page = await context.newPage()
  await page.goto(BASE + url, { waitUntil: 'networkidle' })
  await sleep(1200)
  if (prepare) await prepare(page)
  await sleep(600)
  await page.screenshot({ path: `${OUT}${name}.png` })
  await context.close()
  console.log(name)
}

await shot('lobby', { width: 1100, height: 500, path: '/' })
await shot('room', { width: 1100, height: 760, path: '/r/main-hall' })
await shot('room-phone', { width: 414, height: 800, path: '/r/lightning' })
await shot('stage', { width: 1280, height: 720, path: '/r/main-hall/stage' })
await shot('admin', {
  width: 860,
  height: 940,
  path: '/admin',
  prepare: async (page) => {
    await page.fill('input[type="password"]', ADMIN)
    await page.click('button[type="submit"]')
    await page.waitForSelector('text=Reset for next talk')
  },
})

await browser.close()
