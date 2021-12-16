import { JSONSchema } from 'objection'
import BaseModel from './base'
import { v4 as uuidv4 } from 'uuid'
import { ParamSchema } from 'aejo'

export type IocType = 'fqdn' | 'ip' | 'literal'

export interface IocAttributes {
  id?: string
  type: IocType
  value: string
  enabled: boolean
  created_at?: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of IOC',
    type: 'string',
    format: 'uuid',
  },
  type: {
    description: 'IOC Type',
    type: 'string',
    enum: ['fqdn', 'ip', 'literal'],
  },
  value: {
    description: 'IOC Value',
    type: 'string',
    format: 'regex',
  },
  enabled: {
    description: 'Active IOC',
    type: 'boolean',
  },
  created_at: {
    description: 'Created Date',
    type: 'string',
    format: 'date-time',
  },
}

export default class Ioc extends BaseModel<IocAttributes> {
  id!: string
  /** IOC Type */
  type!: IocType
  /** IOC value */
  value!: string
  enabled: boolean
  created_at: Date

  static get tableName(): string {
    return 'iocs'
  }

  $beforeInsert(): void {
    this.created_at = new Date()
    this.id = uuidv4()
  }

  static updateAble(): Array<keyof IocAttributes> {
    return ['type', 'value', 'enabled']
  }

  static selectAble(): Array<keyof IocAttributes> {
    return ['id', 'type', 'value', 'enabled', 'created_at']
  }

  static insertAble(): Array<keyof IocAttributes> {
    return ['type', 'value', 'enabled']
  }

  static build(o: Partial<IocAttributes>): Ioc {
    return Ioc.fromJson(o)
  }

  static get jsonSchema(): JSONSchema {
    return {
      type: 'object',
      required: ['type', 'value', 'enabled'],
      properties: {
        type: {
          type: 'string',
          enum: ['fqdn', 'ip', 'literal'],
        },
        value: {
          type: 'string',
          minLength: 2,
        },
        enabled: {
          type: 'boolean',
        },
      },
    }
  }
}
