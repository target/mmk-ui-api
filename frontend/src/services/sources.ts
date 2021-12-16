/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'
import { SecretAttributes } from './secrets'

export type SecretSelect = Pick<SecretAttributes, 'id' | 'name' | 'type'>

export interface SourceAttributes {
  id: string
  /* name of source */
  name: string
  /* source to run */
  value: string
  created_at: Date
  secrets?: SecretSelect[]
}

export interface NewSourceRequest {
  source: {
    name: string
    value: string
    secret_ids?: string[]
  }
}

type EagerLoad = 'scans' | 'sites' | 'secrets'

interface SourceListRequest extends ListRequest<SourceAttributes> {
  eager?: Array<EagerLoad>
  no_test?: boolean
}

export interface NewTestSourceResult {
  scan_id: string
  source_id: string
}

const list = async (params?: SourceListRequest) =>
  axios.get<ObjectListResult<SourceAttributes>>('/api/sources', { params })

const view = async (params: { id: string; eager?: Array<EagerLoad> }) =>
  axios.get<SourceAttributes>(`/api/sources/${params.id}`, { params })

const test = async (params?: NewSourceRequest) =>
  axios.post<NewTestSourceResult>('/api/sources/test', params)

const create = async (params?: NewSourceRequest) =>
  axios.post<NewSourceRequest>('/api/sources', params)

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/sources/${params.id}`)

export default {
  list,
  view,
  test,
  create,
  destroy,
}
