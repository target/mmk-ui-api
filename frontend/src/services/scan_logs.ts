/* eslint-disable camelcase */
import axios from 'axios'

export type ScanLogLevels = 'info' | 'error' | 'warning'

export interface ScanLogAttributes {
  id: string
  entry: string
  event: Record<string, unknown>
  scan_id: string
  level: ScanLogLevels
  created_at: Date
}

// duplicate
export interface ObjectListResult<M> {
  results: M[]
  total: number
}

// duplicate
export interface ListRequest<M> {
  fields?: Array<keyof M>
  scan_id?: string
  entry?: string[]
  // valueOf?
  from?: Date
  page?: number
  pageSize?: number
  orderColumn?: keyof M
  orderDirection?: 'asc' | 'desc'
  search?: string
}

export type ObjectDistinctResult<M> = Array<Record<keyof M, string>>

/**
 * list
 * returns an `ObjectListResult` of `ScanLogs`
 */
const list = async (params?: ListRequest<ScanLogAttributes>) =>
  axios.get<ObjectListResult<ScanLogAttributes>>('/api/scan_logs', { params })

/**
 * distinct
 * returns array of distinct `column` values for a given scan
 */
const distinct = async (params: {
  column: keyof ScanLogAttributes
  id: string
}) =>
  axios.get<ObjectDistinctResult<ScanLogAttributes>>(
    `/api/scan_logs/${params.id}/distinct`,
    { params: { column: params.column } }
  )

export default {
  list,
  distinct,
}
