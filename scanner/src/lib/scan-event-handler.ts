import {
  ScanEvent,
  ScanEventType,
  ScanEventPayload,
  EventResult,
  RuleAlert,
} from '@merrymaker/types'
import { Rule } from '../rules/base'
import { JobOptions, Queue } from 'bull'

import logger from '../loaders/logger'

export interface RuleJobData {
  rule: string
  event: ScanEvent
}

export interface RuleJob {
  name: string
  data: RuleJobData
  opts?: JobOptions
}

export type EventHandlerFunction = (
  payload: ScanEventPayload
) => Promise<EventResult[]>

// Scan Event Handler
export default class ScanEventHandler {
  // Determine type
  promiseMap: Record<ScanEventType, Rule[]>
  byName: Map<string, Rule>
  constructor() {
    this.promiseMap = {} as Record<ScanEventType, Rule[]>
    this.byName = new Map<string, Rule>()
  }
  use(st: ScanEventType, handler: Rule): void {
    if (!this.promiseMap[st]) {
      this.promiseMap[st] = []
    }
    this.promiseMap[st].push(handler)
    this.byName.set(handler.ruleDetails.name, handler)
  }
  async scheduleRules(se: ScanEvent, queue: Queue): Promise<void> {
    if (this.promiseMap[se.type]) {
      const jobs: RuleJob[] = []
      this.promiseMap[se.type].forEach((rule) => {
        logger.info(`scheduling rule ${rule.ruleDetails.name}`)
        jobs.push({
          name: 'rule-job',
          data: {
            rule: rule.ruleDetails.name,
            event: se,
          },
          opts: {
            removeOnComplete: true,
          },
        })
      })
      const res = await queue.addBulk(jobs)
      logger.info(`Add Bulk Result ${res[0].name}`)
    } else {
      logger.debug(`no handler for ${se.type}`)
    }
  }
  async process(rj: RuleJobData): Promise<RuleAlert[]> {
    if (this.byName.has(rj.rule)) {
      return this.byName.get(rj.rule).process(rj.event.payload)
    } else {
      return Promise.reject(`no matching rule for ${rj.rule}`)
    }
  }
}
