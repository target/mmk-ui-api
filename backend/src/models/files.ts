import { Model } from 'objection'
import { v4 as uuidv4 } from 'uuid'
import Scan from './scans'

import BaseModel from './base'

export interface FileAttributes {
  id?: string
  scan_id: string
  created_at?: Date
  url: string
  filename: string
  headers: unknown
  sha256: string
}

export default class File extends BaseModel<FileAttributes> {
  id!: string
  scan_id: string
  created_at: Date
  url: string
  filename: string
  headers: unknown
  sha256: string

  static relationMappings = {
    scan: {
      relation: Model.BelongsToOneRelation,
      modelClass: Scan,
      join: {
        from: 'files.scan_id',
        to: 'scan.id',
      },
    },
  }

  static get tableName(): string {
    return 'files'
  }

  static selectAble(): Array<keyof FileAttributes> {
    return ['id', 'scan_id', 'filename', 'sha256', 'created_at', 'url']
  }

  static updateAble(): Array<keyof FileAttributes> {
    return []
  }

  static insertAble(): Array<keyof FileAttributes> {
    return []
  }

  $beforeInsert(): void {
    this.id = uuidv4()
  }
}
