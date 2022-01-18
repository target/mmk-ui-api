import bcrypt from 'bcrypt'
import Objection, {JSONSchema} from 'objection'

import { v4 as uuidv4 } from 'uuid'
import { ParamSchema } from 'aejo'
import BaseModel from './base'

export type UserRole = 'user' | 'transport' | 'admin'

export interface UserAttributes {
  id?: string
  login: string
  password?: string
  password_hash: string
  role: UserRole
  created_at?: Date
  updated_at?: Date
}

export const Schema: { [prop: string]: ParamSchema } = {
  id: {
    description: 'ID of User',
    type: 'string',
    format: 'uuid',
  },
  login: {
    description: 'User Login',
    type: 'string',
  },
  role: {
    description: 'User Role',
    type: 'string',
    enum: ['user', 'transport', 'admin'],
  },
  password: {
    description: 'User Password',
    type: 'string',
    minLength: 8,
    maxLength: 32,
  },
  password_hash: {
    description: 'User Password Hash',
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
  },
}

export default class User extends BaseModel<UserAttributes> {
  id!: string
  login: string
  password?: string
  password_hash: string
  role: UserRole
  created_at: Date
  updated_at?: Date

  public static tableName = 'users'

  public static jsonSchema = {
    type: 'object',
    properties: {
      id: Schema.id,
      login: Schema.login,
      role: Schema.role,
      password_hash: Schema.password_hash,
      password: { type: 'virtual' },
    } as JSONSchema['properties'],
  }

  async $beforeInsert(this: UserAttributes): Promise<void> {
    this.id = uuidv4()
    this.password_hash = await bcrypt.hash(this.password, 10)
    this.created_at = new Date()
  }

  async $beforeUpdate(this: UserAttributes): Promise<void> {
    if (this.password) {
      this.password_hash = await bcrypt.hash(this.password, 10)
    }
    this.updated_at = new Date()
  }

  async $afterUpdate(this: UserAttributes): Promise<void> {
    delete this.password_hash
  }

  static selectAble(): Array<keyof UserAttributes> {
    return ['id', 'login', 'role', 'created_at', 'updated_at']
  }

  static build(o: Partial<UserAttributes>): User {
    return User.fromJson(o)
  }

  async checkPassword(password: string): Promise<boolean> {
    return bcrypt.compare(password, this.password_hash)
  }

  $formatDatabaseJson(json: UserAttributes): Objection.Pojo {
    const outJSON = super.$formatDatabaseJson(json)
    delete outJSON.password
    return outJSON
  }
}
