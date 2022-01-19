import BaseModel from './base'
import { v4 as uuidv4 } from 'uuid'
import { ParamSchema } from 'aejo'

export interface SeenStringAttributes {
  id?: string
  key: string
  type: string
  created_at: Date
  last_cached?: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Seen String',
    type: 'string',
    format: 'uuid',
  },
  type: {
    description: 'Key type',
    type: 'string',
  },
  key: {
    description: 'Key value',
    type: 'string',
  },
  created_at: {
    description: 'Created Dated',
    type: 'string',
    format: 'date-time',
  },
  last_cached: {
    description: 'Date last seen in cache',
    type: 'string',
    format: 'date-time',
  },
}

export default class SeenString extends BaseModel<SeenStringAttributes> {
  id!: string
  key: string
  type: string
  created_at: Date
  last_cached?: Date

  static get tableName(): string {
    return 'seen_strings'
  }

  $beforeInsert(): void {
    this.created_at = new Date()
    this.last_cached = new Date()
    this.id = uuidv4()
  }

  static selectAble(): Array<keyof SeenStringAttributes> {
    return ['id', 'key', 'created_at', 'type', 'last_cached']
  }

  static insertAble(): Array<keyof SeenStringAttributes> {
    return ['key', 'created_at', 'type', 'last_cached']
  }

  static updateAble(): Array<keyof SeenStringAttributes> {
    return ['last_cached', 'type']
  }

  static build(o: Partial<SeenStringAttributes>): SeenString {
    return SeenString.fromJson(o)
  }
}
