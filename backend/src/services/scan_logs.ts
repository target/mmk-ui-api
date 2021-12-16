import { Job } from 'bull'
import LRUCache from 'lru-native2'
import { ScanLog, Scan, Alert } from '../models/'
import { EventEmitter } from 'events'
import MerryMaker from '@merrymaker/types'
import Queues from '../jobs/queues'
import { Modifier } from 'objection'

const oneHour = 1000 * 60 * 60

const siteScanCache = new LRUCache({
  maxElements: 255,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0,
})
const events = new EventEmitter()

async function work(job: Job<MerryMaker.EventResult>): Promise<ScanLog> {
  const evt = job.data

  if (evt.entry === 'rule-alert') {
    // ALERT!
    try {
      await handleAlert(evt as MerryMaker.RuleAlertEvent)
    } catch (e) {
      console.error('ALERT INSERT', e)
      throw new e()
    }
  }

  const logEntry = await ScanLog.query().insert({
    created_at: new Date(),
    entry: evt.entry,
    level: evt.level,
    scan_id: evt.scan_id,
    event: evt.event,
  })
  events.emit('new-entry', logEntry)
  return logEntry
}

const view = async (id: string): Promise<ScanLog> =>
  ScanLog.query().findById(id).skipUndefined().throwIfNotFound()

const distinct = async (
  column: string,
  where?: Modifier
): Promise<Partial<ScanLog[]>> => ScanLog.query().distinct(column).modify(where)

const handleAlert = async (
  logEvent: MerryMaker.RuleAlertEvent
): Promise<void> => {
  if (!logEvent.event.alert) {
    return
  }
  let site_id = siteScanCache.get(logEvent.scan_id)
  if (!site_id) {
    const res = await Scan.query().findById(logEvent.scan_id)
    site_id = res.site_id
  }
  if (!site_id || typeof site_id !== 'string') {
    return
  }
  siteScanCache.set(logEvent.scan_id, site_id)
  if (logEvent.event.alert) {
    await Queues.alertQueue.add(
      {
        level: 'info',
        entry: 'rule-alert',
        scan_id: logEvent.scan_id,
        event: logEvent.event,
      },
      {
        removeOnComplete: true,
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
    created_at: new Date(),
  })
}

export default {
  events,
  work,
  view,
  distinct,
  siteScanCache,
}
