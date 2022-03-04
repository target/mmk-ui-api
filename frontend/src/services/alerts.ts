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
  rule?: string[]
  search?: string
  eager?: Array<EagerLoad>
}

type AlertAggRequest = {
  interval_hours?: number
  start_time?: Date
  end_time?: Date
}

type AlertAggResult = {
  rows: Array<{ hours: string; count: number }> | Array<never>
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

const agg = async (params?: AlertAggRequest) =>
  axios.get<AlertAggResult>('/api/alerts/agg', { params })


export default {
  agg,
  list,
  view,
  destroy,
  distinct
}
