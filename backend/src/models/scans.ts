import { Model } from 'objection'
import { v4 as uuidv4 } from 'uuid'
import Site from './sites'
import Source from './sources'
import ScanLogs from './scan_logs'
import { Schema as SourceSchema } from './sources'

import BaseModel from './base'
import { ParamSchema } from 'aejo'

export interface ScanAttributes {
  id?: string
  site_id?: string
  source_id: string
  source?: Source
  created_at?: Date
  state: string
  test?: boolean
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Scan',
    type: 'string',
    format: 'uuid',
  },
  site_id: {
    description: 'ID of originating site',
    type: 'string',
    format: 'uuid',
  },
  source_id: {
    description: 'ID of source used in the scan',
    type: 'string',
    format: 'uuid',
  },
  source: {
    description: 'Source used during scan (eager loaded)',
    type: 'object',
    properties: SourceSchema,
    nullable: true,
  },
}

export default class Scan extends BaseModel<ScanAttributes> {
  id!: string
  site_id?: string
  source_id: string
  created_at: Date
  source: Source
  test?: boolean
  state: string

  static relationMappings = {
    site: {
      relation: Model.BelongsToOneRelation,
      modelClass: Site,
      join: {
        from: 'scans.site_id',
        to: 'sites.id',
      },
    },
    logs: {
      relation: Model.HasManyRelation,
      modelClass: ScanLogs,
      join: {
        from: 'scans.id',
        to: 'scan_logs.scan_id',
      },
    },
    source: {
      relation: Model.HasOneRelation,
      modelClass: Source,
      join: {
        from: 'scans.source_id',
        to: 'sources.id',
      },
    },
  }

  static get tableName(): string {
    return 'scans'
  }

  static selectAble(): Array<keyof ScanAttributes> {
    return ['id', 'site_id', 'test', 'created_at', 'state', 'source_id']
  }

  static updateAble(): Array<keyof ScanAttributes> {
    return []
  }

  static insertAble(): Array<keyof ScanAttributes> {
    return []
  }

  $beforeInsert(): void {
    this.id = uuidv4()
    if (this.created_at === null) {
      this.created_at = new Date()
    }
  }
}
