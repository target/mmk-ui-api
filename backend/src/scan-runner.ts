import Queues from './jobs/queues'
import { URL } from 'url'
import ScanService from './services/scan'
import { Site, Source } from './models'

const sourceURL = process.argv[2]
const dURL = new URL(sourceURL)
const source = `
    await page.goto('${sourceURL}', { waitUntil: 'domcontentloaded' })
    `
;(async () => {
  await Queues.scannerQueue.isReady()
  const exists = await Source.query().findOne({
    value: source,
  })
  if (exists) {
    await Site.query().where({ source_id: exists.id }).del()
    await exists.$query().del()
  }
  const sourceInst = await Source.query().insertAndFetch({
    name: dURL.hostname,
    value: source,
    created_at: new Date(),
  })
  const siteInst = await Site.query().insertAndFetch({
    name: dURL.hostname,
    active: true,
    run_every_minutes: 0,
    source_id: sourceInst.id,
    created_at: new Date(),
    updated_at: new Date(),
  })
  const runnable = await Site.query().findById(siteInst.id)
  const res = await ScanService.schedule(Queues.scannerQueue, {
    site: runnable,
  })
  console.log('Scheduled...')
  await res.job.finished()
  console.log('Finished')
  process.exit(0)
})()
