import BaseModel from './base'
import { v4 as uuidv4 } from 'uuid'
import { ParamSchema } from 'aejo'

export const AllowListType = [
  'fqdn',
  'ip',
  'literal',
  'ioc-payload-domain',
  'google-analytics',
  'referrer'
]

export interface AllowListAttributes {
  id?: string
  type: typeof AllowListType[number]
  key: string
  created_at?: Date
  updated_at?: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Allow List',
    type: 'string',
    format: 'uuid'
  },
  type: {
    description: 'Key type',
    type: 'string',
    enum: AllowListType
  },
  key: {
    description: 'Matching key pattern',
    type: 'string',
    format: 'regex'
  },
  created_at: {
    description: 'Created Date',
    type: 'string',
    format: 'date-time'
  },
  updated_at: {
    description: 'Updated Date',
    type: 'string',
    format: 'date-time'
  }
}

export default class AllowList extends BaseModel<AllowListAttributes> {
  id!: string
  key: string
  type: string
  created_at: Date
  updated_at?: Date

  static get tableName(): string {
    return 'allow_list'
  }

  $beforeInsert(): void {
    this.id = uuidv4()
    this.created_at = new Date()
  }

  $beforeUpdate(): void {
    this.updated_at = new Date()
  }

  static selectAble(): Array<keyof AllowListAttributes> {
    return ['id', 'key', 'created_at', 'type', 'updated_at']
  }

  static insertAble(): Array<keyof AllowListAttributes> {
    return ['key', 'created_at', 'type', 'updated_at']
  }

  static updateAble(): Array<keyof AllowListAttributes> {
    return ['updated_at', 'type', 'key']
  }

  static build(o: Partial<AllowListAttributes>): AllowList {
    return AllowList.fromJson(o)
  }
}
