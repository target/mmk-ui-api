import { v4 as uuidv4 } from 'uuid'
import { Model } from 'objection'
import BaseModel from './base'
import Secret from './secrets'
import { JSONSchema } from 'objection'
import SourceService from '../services/source'
import { ParamSchema } from 'aejo'

export interface SourceAttributes {
  id?: string
  name: string
  value: string
  test?: boolean
  created_at?: Date
  readonly secrets?: Secret[]
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Source',
    type: 'string',
    format: 'uuid',
  },
  name: {
    description: 'Name of Source',
    type: 'string',
    minLength: 3,
    maxLength: 255,
  },
  value: {
    description: 'Puppeteer code to run',
    type: 'string',
  },
  test: {
    description: 'Run as a test scan',
    type: 'boolean',
  },
  created_at: {
    description: 'Date Source was created',
    type: 'string',
  },
  secrets: {
    description: 'Associated secrets',
    type: 'array',
    // TODO - include items
  },
}

export default class Source extends BaseModel<SourceAttributes> {
  id!: string
  name: string
  value: string
  test?: boolean
  created_at: Date
  secrets?: Secret[]

  static get tableName(): string {
    return 'sources'
  }

  static selectAble(): Array<keyof SourceAttributes> {
    return ['id', 'name', 'test', 'value', 'created_at']
  }

  static updateAble(): Array<keyof SourceAttributes> {
    return []
  }

  static insertAble(): Array<keyof SourceAttributes> {
    return ['name', 'value', 'test']
  }

  $beforeInsert(): void {
    this.id = uuidv4()
    this.created_at = new Date()
  }

  static relationMappings = {
    scans: {
      relation: Model.HasManyRelation,
      modelClass: __dirname + '/scans',
      join: {
        from: 'sources.id',
        to: 'scans.source_id',
      },
    },
    sites: {
      relation: Model.HasOneThroughRelation,
      modelClass: __dirname + '/sites',
      join: {
        from: 'sources.id',
        through: {
          from: 'scans.source_id',
          to: 'scans.site_id',
        },
        to: 'sites.id',
      },
    },
    secrets: {
      relation: Model.ManyToManyRelation,
      modelClass: __dirname + '/secrets',
      join: {
        from: 'sources.id',
        through: {
          from: 'source_secrets.source_id',
          to: 'source_secrets.secret_id',
        },
        to: 'secrets.id',
      },
    },
  }

  // remove cached value
  async $afterDelete(): Promise<number> {
    return SourceService.clearCache(this.id)
  }

  static get jsonSchema(): JSONSchema {
    return {
      type: 'object',
      required: ['name', 'value'],
      properties: {
        name: {
          type: 'string',
          // eslint-disable-next-line no-useless-escape
          pattern: '^[A-Za-z0-9_. ]+$',
          minLength: 3,
          maxLength: 255,
        },
        value: {
          type: 'string',
          minLength: 5,
        },
      },
    }
  }
}
