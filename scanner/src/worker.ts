import {
  EventResult,
  RuleAlert,
  RuleAlertEvent,
  GeneralErrorEvent,
} from '@merrymaker/types'
import Bull, { Job } from 'bull'
import BullWorker from './lib/bull-worker'
import { resolveClient } from './lib/redis'
import { scanHandler } from './rules'
import { RuleJobData } from './lib/scan-event-handler'

import logger from './loaders/logger'

const jsScopeEventQueue = new Bull<EventResult>('browser-event-queue', {
  createClient: resolveClient,
})

const scanLogEventQueue = new Bull<EventResult>('scan-log-queue', {
  createClient: resolveClient,
})
const ruleQueue = new Bull('rule-queue', { createClient: resolveClient })
;(async () => {
  await jsScopeEventQueue.isReady()
  await scanLogEventQueue.isReady()
  await ruleQueue.isReady()
  // temp empty for testing
  await ruleQueue.empty()
})()

jsScopeEventQueue.process(async (job: Job) => {
  if (job.data.type && typeof job.data.type === 'string') {
    await scanLogEventQueue.add(
      {
        scan_id: job.data.scanID,
        entry: job.data.type,
        test: job.data.test,
        level: 'info',
        event: job.data.payload,
      },
      {
        removeOnComplete: true,
        removeOnFail: 25,
      }
    )
    await scanHandler.scheduleRules(job.data, ruleQueue)
  }
})

ruleQueue.on('error', (e) => {
  logger.error({ queue: 'rule', error: e.message })
})

ruleQueue.on('waiting', (jobID: string) => {
  logger.info({ queue: 'rule', status: 'waiting', jobID })
})

// eslint-disable-next-line @typescript-eslint/no-explicit-any
ruleQueue.on('completed', (_job: Job, result: any) => {
  logger.info({ queue: 'rule', status: 'completed', result })
})

const ruleQueueManager = new BullWorker(
  5,
  ruleQueue,
  async (job: Job<RuleJobData>) => {
    try {
      const events = await scanHandler.process(job.data)
      if (events) {
        await scanLogEventQueue.addBulk(
          events.map((evt: RuleAlert) => {
            return {
              data: {
                entry: 'rule-alert',
                level: 'info',
                rule: evt.name,
                scan_id: job.data.event.scanID,
                event: evt,
              } as RuleAlertEvent,
              opts: {
                removeOnComplete: true,
              },
            }
          })
        )
      } else {
        logger.info({ queue: 'rule', status: 'no rule alerts' })
      }
    } catch (e) {
      logger.error({ queue: 'rule', error: e.message })
      await scanLogEventQueue.add(
        {
          entry: 'error',
          level: 'error',
          scan_id: job.data.event.scanID,
          event: {
            message: e.message,
          },
        } as GeneralErrorEvent,
        { removeOnComplete: true }
      )
    }
  }
)

ruleQueueManager.on('info', (msg) => {
  logger.info(`rules queue manager info - ${msg}`, msg)
})
ruleQueueManager.on('error', (err) => {
  logger.error(`rules queue manager error (${err.message})`)
})
// eslint-disable-next-line @typescript-eslint/explicit-function-return-type
;(async () => {
  logger.info('starting!!!')
  setInterval(async () => {
    const total = await ruleQueue.count()
    logger.debug(`Rule Queue Count ${total}`)
  }, 5000)

  await ruleQueueManager.poll()
  logger.info('started')
})()
