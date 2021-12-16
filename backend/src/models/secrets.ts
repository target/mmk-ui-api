import BaseModel from './base'
import { v4 as uuidv4 } from 'uuid'

import { config } from 'node-config-ts'
import { JSONSchema, Model } from 'objection'
import { redisClient } from '../repos/redis'
import { ParamSchema } from 'aejo'

export type SecretTypes = 'manual' | 'external' | 'qt'

export interface SecretAttributes {
  id?: string
  name: string
  type: SecretTypes
  value: string
  created_at?: Date
  updated_at?: Date
}

type SecretAttributesArr = Array<keyof SecretAttributes>

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of Secret',
    type: 'string',
    format: 'uuid',
  },
  name: {
    description: 'Name of secret',
    type: 'string',
  },
  type: {
    description: 'Type of Secret',
    type: 'string',
    enum: ['manual', 'external'],
  },
  value: {
    description: 'Secret Value',
    type: 'string',
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
    nullable: true,
  },
}

export default class Secret extends BaseModel<SecretAttributes> {
  id!: string
  type: SecretTypes
  value: string
  name: string
  created_at: Date
  updated_at?: Date

  static get tableName(): string {
    return 'secrets'
  }

  async $beforeInsert(): Promise<void> {
    this.id = uuidv4()
    this.created_at = new Date()
    if (config.quantumTunnel.enabled === 'true' && this.type === 'qt') {
      this.value = await redisClient.get('qtToken')
    }
  }

  async $beforeUpdate(): Promise<void> {
    this.updated_at = new Date()
    if (config.quantumTunnel.enabled === 'true' && this.type === 'qt') {
      this.value = await redisClient.get('qtToken')
    }
  }

  static selectAble(): SecretAttributesArr {
    return ['id', 'name', 'type', 'value', 'created_at', 'updated_at']
  }

  static insertAble(): SecretAttributesArr {
    return ['name', 'type', 'value', 'updated_at']
  }

  static updateAble(): SecretAttributesArr {
    return ['updated_at', 'type', 'value']
  }

  static build(o: Partial<SecretAttributes>): Secret {
    return Secret.fromJson(o)
  }

  static get jsonSchema(): JSONSchema {
    return {
      type: 'object',
      required: ['name', 'type'],
      properties: {
        type: {
          type: 'string',
          enum: ['manual', 'qt'],
        },
        name: {
          type: 'string',
          pattern: '[A-Za-z_0-9]+',
          minLength: 2,
          maxLength: 64,
        },
      },
    }
  }

  static relationMappings = {
    sources: {
      relation: Model.ManyToManyRelation,
      modelClass: __dirname + '/sources',
      join: {
        from: 'secrets.id',
        through: {
          from: 'source_secrets.secret_id',
          to: 'source_secrets.source_id',
        },
        to: 'sources.id',
      },
    },
  }
}
