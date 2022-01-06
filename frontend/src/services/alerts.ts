/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest, ObjectDistinctResult } from './index'

type EagerLoad = 'site'

export interface AlertAttributes {
  id: string
  rule: string
  message: string
  context?: Record<string, unknown>
  scan_id?: string
  site_id?: string
  site?: { name: string }
  created_at: Date
}

interface AlertListRequest extends ListRequest<AlertAttributes> {
  site_id?: string
  scan_id?: string
  eager?: Array<EagerLoad>
}

const list = async (params?: AlertListRequest) =>
  axios.get<ObjectListResult<AlertAttributes>>('/api/alerts', { params })

const view = async (params: { id: string }) =>
  axios.get<AlertAttributes>(`/api/alerts/${params.id}`)

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/scans/${params.id}`)

const distinct = async (params: { column: keyof AlertAttributes }) =>
  axios.get<ObjectDistinctResult<AlertAttributes>>('/api/alerts/distinct', {
    params
  })

export default {
  list,
  view,
  destroy,
  distinct
}
