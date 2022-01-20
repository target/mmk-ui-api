import { Job } from 'bull'
import { Modifier, QueryBuilder } from 'objection'
import LRUCache from 'lru-native2'
import ScanService from '../services/scan'
import SiteService from '../services/site'
import { ScanLog, Scan, Alert } from '../models/'
import { EventEmitter } from 'events'
import MerryMaker, { EventMessage } from '@merrymaker/types'
import Queues from '../jobs/queues'
import logger from '../loaders/logger'

const oneHour = 1000 * 60 * 60

const siteScanCache = new LRUCache({
  maxElements: 255,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0
})

const events = new EventEmitter()

async function work(
  job: Job<MerryMaker.EventResult | MerryMaker.RuleAlertEvent>
): Promise<ScanLog> {
  const evt = job.data
  if (evt.entry === 'rule-alert') {
    // ALERT!
    try {
      await handleAlert(evt as MerryMaker.RuleAlertEvent)
    } catch (e) {
      logger.error('ALERT INSERT', e)
      throw new Error(e)
    }
  } else if (evt.entry === 'complete') {
    await ScanService.updateState(evt.scan_id, 'completed')
  } else if (evt.entry === 'active') {
    await ScanService.updateState(evt.scan_id, 'active')
  } else if (evt.entry === 'failed') {
    await handleFailed(job)
  }

  const logEntry = await ScanLog.query().insert({
    created_at: new Date(),
    entry: evt.entry,
    level: evt.level,
    scan_id: evt.scan_id,
    event: evt.event
  })
  events.emit('new-entry', logEntry)
  return logEntry
}

async function handleFailed(job: Job<MerryMaker.EventResult>) {
  const scan = await ScanService.updateState(
    job.data.scan_id,
    'failed',
    job.failedReason
  )
  const site = await SiteService.view(scan.site_id)
  Queues.alertQueue.add(
    {
      level: 'error',
      entry: 'error',
      scan_id: job.data.scan_id,
      event: {
        message: `${site.name}/${job.queue.name} - ${
          (job.data.event as EventMessage).message
        }`
      }
    },
    {
      removeOnComplete: true,
      attempts: 3
    }
  )
}

const view = async (id: string): Promise<ScanLog> =>
  ScanLog.query()
    .findById(id)
    .skipUndefined()
    .throwIfNotFound()

const distinct = async (
  column: string,
  where?: Modifier
): Promise<Partial<ScanLog[]>> =>
  ScanLog.query()
    .distinct(column)
    .modify(where)

/**
 * countByScanID
 *
 * Fetch total number of matching ScanLogs
 * by scan.id. Allow for additional filtering
 * on `entry` and `whereBuilder`
 */
const countByScanID = (
  id: string,
  entry: string,
  whereBuilder?: Modifier<QueryBuilder<ScanLog, ScanLog[]>>
): Promise<number> =>
  ScanLog.query()
    .where('scan_id', id)
    .where('entry', entry)
    .modify(whereBuilder)
    .skipUndefined()
    .resultSize()

/**
 * handleAlert
 *
 * Handles rule alerts from scanner.
 *
 * Adds the alert to the AlertQueue, and inserts a new Alert
 * record for the UI.
 *
 */
const handleAlert = async (
  logEvent: MerryMaker.RuleAlertEvent
): Promise<void> => {
  if (!logEvent.event.alert) {
    return
  }
  // lookup site ID from siteScan Cache (avoid DB lookups)
  let site_id = siteScanCache.get(logEvent.scan_id)

  // cache-miss, lookup scan in the DB
  if (!site_id) {
    const res = await Scan.query().findById(logEvent.scan_id)
    site_id = res.site_id
  }
  if (site_id || typeof site_id !== 'string') {
    return
  }
  // read-through cache
  siteScanCache.set(logEvent.scan_id, site_id)
  if (logEvent.event.alert) {
    await Queues.alertQueue.add(
      {
        level: 'info',
        entry: 'rule-alert',
        scan_id: logEvent.scan_id,
        event: logEvent.event
      },
      {
        removeOnComplete: true
        // need to split out goAlert and kakfa sending
        //attempts: 3,
      }
    )
  }
  // Need to alert AlertService
  await Alert.query().insert({
    rule: logEvent.rule,
    message: logEvent.event.message,
    context: logEvent.event.context,
    scan_id: logEvent.scan_id,
    site_id,
    created_at: new Date()
  })
}

export default {
  events,
  work,
  view,
  distinct,
  countByScanID,
  siteScanCache
}
