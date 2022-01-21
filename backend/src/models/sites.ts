import BaseModel from './base'
import { Model, JSONSchema } from 'objection'
import { v4 as uuidv4 } from 'uuid'
import Scan from './scans'
import Source from './sources'
import { ParamSchema } from 'aejo'

// TODO - sanitize / strip HTML on create/update
export interface SiteAttributes {
  id?: string
  name: string
  last_run?: Date
  active: boolean
  run_every_minutes: number
  source_id: string
  created_at?: Date
  updated_at?: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Site',
    type: 'string',
    format: 'uuid',
  },
  name: {
    description: 'Name of Site',
    type: 'string',
  },
  active: {
    description: 'Site is active',
    type: 'boolean',
  },
  last_run: {
    description: 'Last scan of Site',
    type: 'string',
    format: 'date-time',
  },
  run_every_minutes: {
    description: 'Number of minutes to wait between scans',
    type: 'integer',
  },
  source_id: {
    description: 'Reference source to run',
    type: 'string',
    format: 'uuid',
  },
  created_at: {
    description: 'Created Date',
    type: 'string',
    format: 'date-time',
  },
  updated_at: {
    description: 'Updated Date',
    type: 'string',
    format: 'date-time',
  },
}

export default class Site extends BaseModel<SiteAttributes> {
  id!: string
  /** Name of Site */
  name!: string
  /** Last run date */
  last_run: Date
  /** Source ID */
  source_id: string
  run_every_minutes: number
  /** Site will run on next scheduled interval */
  active!: boolean
  created_at: Date
  updated_at: Date

  static updateAble(): Array<keyof Site> {
    return ['name', 'active', 'source_id', 'run_every_minutes', 'last_run']
  }

  static selectAble(): Array<keyof Site> {
    return [
      'id',
      'name',
      'source_id',
      'run_every_minutes',
      'last_run',
      'active',
      'created_at',
      'updated_at',
    ]
  }
  static insertAble(): Array<keyof Site> {
    return ['name', 'active', 'source_id', 'run_every_minutes']
  }

  static build(o: Partial<Site>): Site {
    return Site.fromJson(o)
  }

  static relationMappings = {
    scans: {
      relation: Model.HasManyRelation,
      modelClass: Scan,
      join: {
        from: 'sites.id',
        to: 'scans.site_id',
      },
    },
    source: {
      relation: Model.HasOneRelation,
      modelClass: Source,
      join: {
        from: 'sites.source_id',
        to: 'sources.id',
      },
    },
  }

  static get tableName(): string {
    return 'sites'
  }

  $beforeInsert(): void {
    this.id = uuidv4()
  }

  static get jsonSchema(): JSONSchema {
    return {
      type: 'object',
      required: ['name', 'active', 'source_id', 'run_every_minutes'],
      properties: {
        name: {
          type: 'string',
          pattern: '^[A-Za-z0-9_. ]+$',
          minLength: 3,
          maxLength: 255,
        },
        active: {
          type: 'boolean',
        },
        source_id: {
          type: 'string',
          pattern:
            '^[0-9a-f]{8}-[0-9a-f]{4}-[0-5][0-9a-f]{3}-[089ab][0-9a-f]{3}-[0-9a-f]{12}$',
        },
        run_every_minutes: {
          type: 'integer',
        },
      },
    }
  }
}
