import { Model } from 'objection'
import { v4 as uuidv4 } from 'uuid'

import Scan from './scans'
import Site from './sites'

import BaseModel from './base'
import { ParamSchema } from 'aejo'

export interface AlertAttributes {
  id?: string
  rule: string
  message: string
  context?: Record<string, unknown>
  scan_id?: string
  site_id?: string
  created_at: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Alert',
    type: 'string',
    format: 'uuid',
  },
  rule: {
    description: 'Associated Rule',
    type: 'string',
    enum: [
      'ioc.payload',
      'ioc.domain',
      'unknown.domain',
      'google.analytics',
      'yara',
      'domain.via.websocket',
    ],
  },
  message: {
    description: 'Alert Message',
    type: 'string',
  },
  context: {
    description: 'Alert context',
    type: 'object',
  },
  scan_id: {
    description: 'ID of related Scan',
    type: 'string',
    format: 'uuid',
  },
  site_id: {
    description: 'ID of related Site',
    type: 'string',
    format: 'uuid',
  },
  created_at: {
    description: 'Datetime of Alert',
    type: 'string',
    format: 'date-time',
  },
}

export default class Alert extends BaseModel<AlertAttributes> {
  id!: string
  rule: string
  message: string
  context?: Record<string, unknown>
  scan_id?: string
  site_id?: string
  created_at: Date

  static relationMappings = {
    scan: {
      relation: Model.BelongsToOneRelation,
      modelClass: Scan,
      join: {
        from: 'alerts.scan_id',
        to: 'scans.id',
      },
    },
    site: {
      relation: Model.BelongsToOneRelation,
      modelClass: Site,
      join: {
        from: 'alerts.site_id',
        to: 'sites.id',
      },
    },
  }

  static get tableName(): string {
    return 'alerts'
  }

  static selectAble(): Array<keyof AlertAttributes> {
    return [
      'id',
      'rule',
      'message',
      'scan_id',
      'site_id',
      'created_at',
      'context',
    ]
  }

  static updateAble(): Array<keyof AlertAttributes> {
    return []
  }

  static insertAble(): Array<keyof AlertAttributes> {
    return []
  }

  $beforeInsert(): void {
    this.id = uuidv4()
  }
}
