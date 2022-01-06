/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest, ObjectDistinctResult } from './index'

export interface SeenStringAttributes {
  id: string
  key: string
  type: string
  created_at: Date
  last_cached?: Date
}

export type SeenStringTypes = 'domain' | 'hash' | 'url' | 'email'

export interface SeenStringListRequest
  extends ListRequest<SeenStringAttributes> {
  key?: string
  type?: string
  search?: string
}

type SeenStringRequest = Pick<SeenStringAttributes, 'key' | 'type'>

const list = (params?: SeenStringListRequest) =>
  axios.get<ObjectListResult<SeenStringAttributes>>('/api/seen_strings', {
    params
  })

const view = async (params: { id: string }) =>
  axios.get<SeenStringAttributes>(`/api/seen_strings/${params.id}`)

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/seen_strings/${params.id}`)

const create = async (params: SeenStringRequest) =>
  axios.post<SeenStringAttributes>('/api/seen_strings', { seen_string: params })

const update = async (id: string, params: SeenStringRequest) =>
  axios.put<SeenStringAttributes>(`/api/seen_strings/${id}`, {
    seen_string: params
  })

const distinct = async (params: { column: keyof SeenStringAttributes }) =>
  axios.get<ObjectDistinctResult<SeenStringAttributes>>(
    '/api/seen_strings/distinct',
    {
      params
    }
  )

export default {
  list,
  distinct,
  destroy,
  view,
  create,
  update
}
