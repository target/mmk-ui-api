import BaseModel from './base'

export interface SourceSecretAttributes {
  source_id: string
  secret_id: string
}

export default class SourceSecret extends BaseModel<SourceSecretAttributes> {
  source_id: string
  secret_id: string

  static get tableName(): string {
    return 'source_secrets'
  }

  static get idColumn(): string[] {
    return ['source_id', 'secret_id']
  }
}
