import { Scan, ScanLog, Site, Source } from '../models'
import { Queue, Job } from 'bull'
import logger from '../loaders/logger'
import { ScanLogLevels } from '../models/scan_logs'
import { QueryBuilder, raw } from 'objection'
import MerryMaker from '@merrymaker/types'

type ScheduledScan = { scan: Scan; job: Job }
type ScanScheduleOptions = {
  site?: Site
  source?: Source
  test?: boolean
}

const isActive = async (id: string): Promise<boolean> => {
  const record = await Scan.query().findById(id)
  return record && record.state === 'active'
}

const isBulkActive = async (ids: string[]): Promise<boolean> => {
  const actives = await Scan.query()
    .whereIn('id', ids)
    .andWhere({ state: 'active' })
  return actives && actives.length > 0
}

const view = async (id: string): Promise<Scan> =>
  Scan.query()
    .findById(id)
    .skipUndefined()
    .throwIfNotFound()

const destroy = async (id: string): Promise<number> =>
  Scan.query().deleteById(id)

/**
 * schedule
 *   Schedules a scan to run in the `queue`
 *   Creates a new ScanLog Entry with scan details
 */
const schedule = async (
  queue: Queue<MerryMaker.ScanQueueJob>,
  opts: ScanScheduleOptions
): Promise<ScheduledScan> => {
  let options: { site_id?: string; source_id: string; test?: boolean }
  let name = ''
  if (opts.site) {
    options = {
      site_id: opts.site.id,
      source_id: opts.site.source_id
    }
    name = opts.site.name
    // update `last_run` to prevent scheduling
    // again before it completes
    await Site.query()
      .patch({
        last_run: new Date()
      })
      .where('id', opts.site.id)
  } else if (opts.source) {
    options = {
      source_id: opts.source.id
    }
    name = opts.source.name
  }
  const scanInst = await Scan.query().insertAndFetch({
    ...options,
    created_at: new Date(),
    state: 'scheduled',
    test: opts.test
  })
  const jobInst = await queue.add(
    'scan-queue',
    {
      name,
      scan_id: scanInst.id,
      source_id: options.source_id,
      test: opts.test
    },
    {
      // manually removed in `jobs`
      removeOnComplete: false,
      // only attempt once for tests
      attempts: opts.test ? 1 : 3,
      // fail after 30 minutes
      timeout: 1000 * 60 * 30,
      removeOnFail: true
    }
  )
  await ScanLog.query().insertAndFetch({
    event: {
      message: `
      Scheduled ${name} to run
      with source_id ${options.source_id} and
      job_id ${jobInst.id}
      `
    },
    entry: 'log-message',
    scan_id: scanInst.id,
    level: 'info',
    created_at: new Date()
  })
  return { scan: scanInst, job: jobInst }
}

/**
 * updateState
 *   Updates the state of a running scan
 *   Appends a Scan Log Entry on state change
 */
const updateState = async (
  scan_id: string,
  state: string,
  err?: string
): Promise<Scan> => {
  logger.info(`Attempting to update ${scan_id} to state "${state}`)
  const scanInst = await Scan.query().findById(scan_id)
  if (scanInst === null) {
    logger.warn(`Unable to find scan with id ${scan_id}`)
    return
  }
  if (scanInst.state === 'completed') {
    logger.warn(
      `Scan state cannot be changed to "${state}" when already complete`
    )
    return
  }
  await scanInst.$query().patch({ state })
  let event = `Status changed to "${state}"`
  const entry = 'log-message'
  let level: ScanLogLevels = 'info'
  if (err) {
    event = `${event} : ${err}`
    level = 'error'
    logger.warn(`Error while running job scan_id ${scan_id}`, {
      error: err
    })
  }
  await ScanLog.query().insert({
    entry,
    event: { message: event },
    level,
    scan_id: scanInst.id,
    created_at: new Date()
  })
  logger.info('Patching site with last_run')
  // update Site last_run
  await Site.query()
    .patch({
      last_run: new Date()
    })
    .where('id', scanInst.site_id)
  return scanInst
}

/**
 * purge
 *
 *  Delete scans older than or equal to `maxDays`
 */
const purge = async (maxDays: number): Promise<number> =>
  Scan.query()
    .delete()
    .where(raw("created_at <= NOW() - INTERVAL '?? days'", [maxDays]))

/**
 * ruleAlertEvent
 *
 * QueryBuilder for fetching scanLog events
 * where { alert: true } exists
 */
const ruleAlertEvent = (builder: QueryBuilder<ScanLog, ScanLog>) =>
  builder.whereRaw('event::jsonb @> ?', [{ alert: true }])

/**
 * purgeTests
 *
 * Delete test scans older than or equal to `maxHours`
 */
const purgeTests = async (maxHours: number): Promise<number> =>
  Scan.query()
    .delete()
    .where(raw("created_at <= NOW() - INTERVAL '?? hours'", [maxHours]))
    .andWhere('test', true)

const bulkDelete = async (ids: string[]): Promise<number> =>
  Scan.query()
    .delete()
    .whereIn('id', ids)

const totalScheduled = async (): Promise<number> =>
  Scan.query()
    .where('state', '=', 'scheduled')
    .resultSize()

export default {
  schedule,
  updateState,
  purge,
  purgeTests,
  bulkDelete,
  totalScheduled,
  isActive,
  view,
  destroy,
  isBulkActive,
  ruleAlertEvent
}
