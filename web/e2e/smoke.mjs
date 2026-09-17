// Browser smoke test for the attendee views.
//
// A unit test cannot tell you that the transcript never appeared because a
// hook threw, or that the stage display renders an empty box. This drives a
// real browser against a real server and asserts that the words a speaker
// said actually reach the screen.
//
//   node e2e/smoke.mjs http://127.0.0.1:8080 <room-id>

import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const base = process.argv[2] ?? 'http://127.0.0.1:8080'
const room = process.argv[3] ?? 'smoke'
const outDir = join(dirname(fileURLToPath(import.meta.url)), 'output')
mkdirSync(outDir, { recursive: true })

// A locally installed browser takes precedence, which is how this runs in a
// sandbox that already has one.
const executablePath = process.env.CHROMIUM_PATH || undefined

const failures = []
const check = (ok, message) => {
  console.log(`${ok ? '  ok  ' : '  FAIL'} ${message}`)
  if (!ok) failures.push(message)
}

const browser = await chromium.launch({
  ...(executablePath ? { executablePath } : {}),
  args: ['--no-sandbox'],
})

/** Opens a page, collects any console errors, and screenshots it. */
async function visit(path, name, viewport, settleMs = 4000) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })
  page.on('pageerror', (e) => consoleErrors.push(e.message))

  await page.goto(base + path, { waitUntil: 'networkidle' })
  await page.waitForTimeout(settleMs)
  await page.screenshot({ path: join(outDir, `${name}.png`) })
  const text = await page.innerText('body')
  await context.close()
  return { text, consoleErrors }
}

/**
 * The stage display is the one nobody looks at closely until it is on a
 * projector in front of three hundred people, so check the two things that
 * have actually gone wrong on one: the controls in the header drawn on top of
 * each other, and a QR code that is present in the markup but never loaded.
 * Both look perfectly fine to any check that only reads the page text.
 */
async function checkStageFurniture(viewport) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  await page.goto(`${base}/r/${room}/stage`, { waitUntil: 'networkidle' })
  // The controls fade out when nobody has touched the laptop, and something
  // invisible cannot be seen to overlap anything. Keep them awake.
  await page.mouse.move(viewport.width / 2, viewport.height / 2)
  await page.waitForTimeout(1500)

  const { boxes, qr } = await page.evaluate(() => {
    // Everything drawn across the top of the screen, gathered by what it is
    // rather than by where it sits in the markup: the exit link used to be a
    // sibling of the header floated over the top of it, and the point of the
    // check is that it must not collide wherever it happens to live.
    const els = new Set([
      ...document.querySelectorAll('.stage-header h1, .stage-meta > *, .stage-exit'),
    ])
    const boxes = [...els].map((el) => {
      const r = el.getBoundingClientRect()
      return { text: el.textContent.trim().slice(0, 20), l: r.left, t: r.top, r: r.right, b: r.bottom }
    })
    const img = document.querySelector('.stage-qr img')
    return {
      boxes,
      qr: img ? { natural: img.naturalWidth, width: img.getBoundingClientRect().width } : null,
    }
  })

  let clash = ''
  for (let i = 0; i < boxes.length; i++) {
    for (let j = i + 1; j < boxes.length; j++) {
      const a = boxes[i]
      const b = boxes[j]
      if (Math.min(a.r, b.r) > Math.max(a.l, b.l) && Math.min(a.b, b.b) > Math.max(a.t, b.t)) {
        clash = `: ${a.text} over ${b.text}`
      }
    }
  }
  check(boxes.length >= 3, `[stage] the header has a title, a badge and an exit link (${boxes.length})`)
  check(clash === '', `[stage] nothing across the top of the screen overlaps${clash}`)

  // naturalWidth is the whole point: a broken image still has a layout box,
  // which is how a small white square passed for a QR code for a whole talk.
  check(qr !== null, '[stage] the QR code is present')
  check(qr !== null && qr.natural > 0, `[stage] the QR code actually loaded (naturalWidth ${qr?.natural})`)
  check(qr !== null && qr.width >= 80, `[stage] the QR code is big enough to scan (${Math.round(qr?.width ?? 0)}px)`)

  await page.screenshot({ path: join(outDir, 'stage-furniture.png') })
  await context.close()
}

/**
 * A stage screen is left running for hours and will outlive a server restart,
 * a flapping Wi-Fi link or a relay being moved. A browser never retries a
 * broken image by itself, so one failed request at the wrong moment used to
 * leave a blank square where the QR code should be for the rest of the talk,
 * with nothing on screen to suggest anything was wrong.
 */
async function checkQRSurvivesAnOutage(viewport) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()

  const downFor = 8000
  const startedAt = Date.now()
  let refused = 0
  await page.route('**/qr.png*', async (route) => {
    if (Date.now() - startedAt < downFor) {
      refused++
      await route.abort('connectionrefused')
    } else {
      await route.continue()
    }
  })

  await page.goto(`${base}/r/${room}/stage`, { waitUntil: 'domcontentloaded' })
  const loaded = async () =>
    page.evaluate(() => document.querySelector('.stage-qr img')?.naturalWidth ?? -1)

  await page.waitForTimeout(3000)
  check(refused > 0 && (await loaded()) === 0, '[stage] the QR code is broken whilst the server is away')

  // Long enough for a retry to fall due once the requests start succeeding.
  await page.waitForTimeout(14000)
  check((await loaded()) > 0, `[stage] the QR code comes back on its own (naturalWidth ${await loaded()})`)

  await context.close()
}

/**
 * The transcript must follow the speaker without the page growing: the
 * newest line stays on screen, and it only stops following when the reader
 * scrolls back to read something.
 */
async function checkFollowsTheSpeaker(path, name, viewport) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  await page.goto(base + path, { waitUntil: 'networkidle' })
  await page.waitForTimeout(3000)

  const state = () =>
    page.evaluate(() => {
      const scroller = document.querySelector('.transcript')
      const lines = [...document.querySelectorAll('.transcript .line')]
      const last = lines[lines.length - 1]?.getBoundingClientRect()
      const doc = document.scrollingElement
      return {
        pageScrolls: doc.scrollHeight > doc.clientHeight + 1,
        overflowing: scroller.scrollHeight > scroller.clientHeight + 1,
        scrollTop: Math.round(scroller.scrollTop),
        atBottom: scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 4,
        lastVisible: last ? last.bottom <= window.innerHeight + 1 && last.top >= 0 : false,
        jumpButton: !!document.querySelector('.jump'),
        lineCount: lines.length,
      }
    })

  const initial = await state()
  check(initial.overflowing, `[${name}] there is more transcript than fits, so following matters`)
  // The page growing instead of the transcript scrolling is what used to
  // carry the newest line off the bottom of the screen.
  check(!initial.pageScrolls, `[${name}] the page itself does not scroll`)
  check(initial.lastVisible, `[${name}] the newest line is on screen`)
  check(!initial.jumpButton, `[${name}] no "jump to live" button while following`)

  await page.waitForTimeout(6000)
  const later = await state()
  check(later.lineCount > initial.lineCount, `[${name}] new lines arrived`)
  check(later.atBottom && later.lastVisible, `[${name}] still following after new lines`)

  // Scroll back, as a reader catching up on a missed sentence would.
  await page.evaluate(() => {
    document.querySelector('.transcript').scrollTop = 200
  })
  await page.waitForTimeout(1500)
  const parked = await page.evaluate(() => Math.round(document.querySelector('.transcript').scrollTop))
  await page.waitForTimeout(6000)
  const held = await state()
  check(Math.abs(held.scrollTop - parked) < 50, `[${name}] scrolling back stops the auto-follow`)
  check(held.jumpButton, `[${name}] "jump to live" offered once scrolled back`)

  await page.click('.jump')
  await page.waitForTimeout(800)
  const resumed = await state()
  check(resumed.atBottom && resumed.lastVisible, `[${name}] "jump to live" returns to the newest line`)

  await context.close()
}

try {
  const lobby = await visit('/', 'lobby', { width: 1280, height: 800 }, 2000)
  check(lobby.consoleErrors.length === 0, `lobby has no console errors ${lobby.consoleErrors.join('; ')}`)
  check(/Smoke Room|smoke/i.test(lobby.text), 'lobby lists the live room')
  check(/live/i.test(lobby.text), 'lobby marks the room as live')

  const roomView = await visit(`/r/${room}`, 'room', { width: 390, height: 844 })
  check(roomView.consoleErrors.length === 0, `room view has no console errors ${roomView.consoleErrors.join('; ')}`)
  check(roomView.text.includes('Smoke Room'), 'room view shows the room title')
  check(/replication|postgres|elephant/i.test(roomView.text), 'room view shows transcript text')
  check(/Download text/i.test(roomView.text), 'room view offers the transcript download')

  const stage = await visit(`/r/${room}/stage`, 'stage', { width: 1920, height: 1080 })
  check(stage.consoleErrors.length === 0, `stage view has no console errors ${stage.consoleErrors.join('; ')}`)
  check(/replication|postgres|elephant/i.test(stage.text), 'stage view shows transcript text')
  await checkStageFurniture({ width: 1920, height: 1080 })
  // A projector with an unhelpful aspect ratio: the header has least room
  // here, so it is where the controls will collide first if they ever do.
  await checkStageFurniture({ width: 1280, height: 720 })
  await checkQRSurvivesAnOutage({ width: 1280, height: 720 })

  const missing = await visit('/r/no-such-room', 'missing', { width: 1280, height: 800 }, 2500)
  check(/No such room/i.test(missing.text), 'an unknown room is reported rather than hanging')

  await checkFollowsTheSpeaker(`/r/${room}`, 'desktop', { width: 1280, height: 800 })
  await checkFollowsTheSpeaker(`/r/${room}`, 'phone', { width: 390, height: 844 })
} finally {
  await browser.close()
}

if (failures.length > 0) {
  console.error(`\n${failures.length} smoke check(s) failed; screenshots are in ${outDir}`)
  process.exit(1)
}
console.log('\nAll smoke checks passed.')
