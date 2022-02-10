import { v4 as uuidv4 } from 'uuid'
import MerryMaker from '@merrymaker/types'

import { stripJSONUnicode } from '../lib/utils'
import BaseModel from './base'
import { ParamSchema } from 'aejo'

export type ScanLogLevels = 'info' | 'error' | 'warning'
export interface ScanLogAttributes {
  id?: string
  entry: MerryMaker.ScanEventType
  event: MerryMaker.ScanEventPayload
  scan_id: string
  level: ScanLogLevels
  created_at: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of ScanLog',
    type: 'string',
    format: 'uuid',
  },
  entry: {
    description: 'Scan entry type',
    type: 'string',
    enum: [
      'page-error',
      'console-message',
      'worker-created',
      'cookie',
      'request',
      'response-error',
      'function-call',
      'script-response',
      'file-download',
      'log-message',
      'screenshot',
      'error',
      'complete',
      'rule-alert',
    ],
  },
  event: {
    description: 'Log entry',
    type: 'object',
  },
  scan_id: {
    type: 'string',
    description: 'Scan ID related to ScanLog',
    format: 'uuid',
  },
  level: {
    type: 'string',
    description: 'Log Level',
    enum: ['info', 'error', 'warning'],
  },
  created_at: {
    type: 'string',
    description: 'Date ScanLog entry was created',
    format: 'date-time',
  },
}

export default class ScanLog extends BaseModel<ScanLogAttributes> {
  id!: string
  entry: MerryMaker.ScanEventType
  scan_id: string
  event: MerryMaker.ScanEventPayload
  level: ScanLogLevels
  created_at: Date

  static get tableName(): string {
    return 'scan_logs'
  }

  static selectAble(): Array<keyof ScanLogAttributes> {
    return ['id', 'entry', 'scan_id', 'level', 'event', 'created_at']
  }

  static updateAble(): Array<keyof ScanLogAttributes> {
    return []
  }

  static insertAble(): Array<keyof ScanLogAttributes> {
    return []
  }

  $beforeInsert(): void {
    this.id = uuidv4()
    if (this.event) {
      this.event = stripJSONUnicode(this.event)
    }
    if (this.created_at === null) {
      this.created_at = new Date()
    }
  }
}
