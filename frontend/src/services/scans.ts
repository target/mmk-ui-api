/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'
import { SiteAttributes } from './sites'
import { SourceAttributes } from './sources'

type EagerLoad = 'sites' | 'sources'

export interface ScanAttributes {
  name: string
  id: string
  site_id: string
  source_id: string
  created_at: Date
  state: string
  test: boolean
  site?: SiteAttributes
  source?: SourceAttributes
}

export interface ScanListRequest extends ListRequest<ScanAttributes> {
  eager?: Array<EagerLoad>
  site_id?: string
  entry?: string[]
  no_test?: boolean
}

const list = async (params?: ScanListRequest) =>
  axios.get<ObjectListResult<ScanAttributes>>('/api/scans', { params })

const view = async (params: { id: string; eager?: EagerLoad[] }) =>
  axios.get<ScanAttributes>(`/api/scans/${params.id}`, {
    params: { eager: params.eager },
  })

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/scans/${params.id}`)

const bulkDelete = async (params: { ids: string[] }) =>
  axios.post('/api/scans/bulk_delete', { scans: { ids: params.ids } })

export default {
  list,
  view,
  destroy,
  bulkDelete,
}
