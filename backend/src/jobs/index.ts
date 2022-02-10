import { config } from 'node-config-ts'
import { createClient } from '../repos/redis'
import logger from '../loaders/logger'
import SiteService from '../services/site'
import ScanService from '../services/scan'
import SourceService from '../services/source'
import AlertService from '../services/alert'
import ScanLogService from '../services/scan_logs'
import SeenStringService from '../services/seen_string'

import Queues from './queues'

import { EventEmitter } from 'events'

EventEmitter.defaultMaxListeners = 15

const redisClient = createClient()

Queues.scannerEventQueue.process(ScanLogService.work)
;(async () => {

  logger.info('Syncing source cache')
  const totalSync = await SourceService.syncCache()
  logger.info(`Synced ${totalSync} sources with cache`)
  await Queues.scannerScheduler.isReady()
  await Queues.localQueue.isReady()
  await Queues.qtSecretRefresh.isReady()
  await Queues.scannerEventQueue.isReady()
  if (config.env === 'test' || config.env === 'development') {
    Queues.scannerQueue.empty()
  }
  Queues.localQueue.empty()
  setInterval(async () => {
    const sQueue = await Queues.scannerQueue.count()
    const ssCount = await Queues.scannerScheduler.count()
    const sECount = await Queues.scannerEventQueue.count()
    await redisClient.set(
      'job-queue',
      JSON.stringify({
        schedule: sQueue,
        event: sECount,
        scanner: sQueue
      })
    )
    logger.info(
      `Schedule Count ${ssCount} / Event Queue ${sECount} / Scanner Queue ${sQueue}`
    )
  }, 5000)
})()

Queues.scannerScheduler.add(
  { run: 1 },
  { repeat: { every: 30000 }, removeOnComplete: true }
)

Queues.localQueue.add(
  'scanner-daily-purge',
  { run: 1 },
  {
    // repeat purge job once every day at 01:00
    repeat: { cron: '0 1 * * *' },
    removeOnComplete: true
  }
)

Queues.localQueue.add(
  'seenStrings-daily-purge',
  { run: 1 },
  {
    // repeat purge job once everyday at 02:00
    repeat: { cron: '0 2 * * *' },
    removeOnComplete: true
  }
)

Queues.localQueue.add(
  'scanner-hourly-purge',
  {
    repeat: { cron: '0 * * * *' },
    removeOnComplete: true
  }
)

// Update job states
Queues.scannerQueue.on('global:active', async jobId => {
  const job = await Queues.scannerQueue.getJob(jobId)
  await ScanService.updateState(job.data.scan_id, 'active')
})

Queues.scannerQueue.on('global:completed', async jobId => {
  try {
    const job = await Queues.scannerQueue.getJob(jobId)
    await ScanService.updateState(job.data.scan_id, 'completed')
    const jstate = await job.getState()
    const isActive = await job.isActive()
    const isComplete = await job.isCompleted()
    logger.info(
      'attemping to remove',
      jstate,
      'isActive',
      isActive,
      'isCompleted',
      isComplete
    )
    return Promise.resolve()
  } catch (e) {
    logger.error(e.message)
    return Promise.reject(e)
  }
})

Queues.scannerQueue.on('global:failed', async jobId => {
  const job = await Queues.scannerQueue.getJob(jobId)
  await ScanService.updateState(job.data.scan_id, 'failed', job.failedReason)
  if (job.data.test) {
    return
  }
  Queues.alertQueue.add(
    {
      level: 'error',
      entry: 'error',
      scan_id: job.data.scan_id,
      event: {
        message: `${job.data.name}/${job.name} - ${job.failedReason}`
      }
    },
    {
      removeOnComplete: true,
      attempts: 3
    }
  )
})

// purge scans 14 days or older, and test scans 6 hours and older
Queues.localQueue.process('scanner-daily-purge', async () => {
  await ScanService.purge(14)
  await ScanService.purgeTests(6)
})

// six months
const MAX_SEEN_STRING_DAYS = 30 * 6

Queues.localQueue.process('seenStrings-daily-purge', async () => {
  logger.info(`Running daily seenString purge (${MAX_SEEN_STRING_DAYS} days)`)
  const total = await SeenStringService.purgeDBCache(MAX_SEEN_STRING_DAYS)
  if (Number.isInteger(total)) {
    logger.info(
      `Removed ${total.toLocaleString()} SeenString records due to cache expiration`
    )
  } else {
    logger.info('Did not find any seenStrings to expire')
  }
})

Queues.localQueue.process('scanner-hourly-purge', () =>
  ScanService.findAndExpire(60)
)

/** Add scans ready to run to the queue */
Queues.scannerScheduler.process(async (_job, done) => {
  try {
    const totalSched = await ScanService.totalScheduled()
    if (totalSched > 20) {
      // attempt to prevent backfill
      logger.warn(`Too many scehduled ${totalSched}, trying again later`)
      return done()
    }
    const runnable = await SiteService.getRunnable()
    logger.debug('found runnable', runnable)
    for (let i = 0; i < runnable.length; i += 1) {
      await ScanService.schedule(Queues.scannerQueue, { site: runnable[i] })
    }
    done()
  } catch (e) {
    done(e)
  }
})

Queues.scannerScheduler.on('completed', job => {
  logger.debug('completed', job.data)
})

Queues.alertQueue.process(async (job, done) => {
  try {
    await AlertService.process(job.data)
    done()
  } catch (e) {
    done(e)
  }
})

// Remove old scans on startup
ScanService.findAndExpire(60)
