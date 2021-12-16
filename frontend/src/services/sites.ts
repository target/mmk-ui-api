/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'

export interface SiteAttributes {
  id: string
  name: string
  last_run: Date
  active: boolean
  run_every_minutes: number
  source_id: string
  created_at: Date
  updated_at: Date
}

export interface SiteRequest {
  id?: string
  name: string
  active: boolean
  run_every_minutes: number
  source_id: string
}

export interface NewSiteResult {
  id: string
}

type SiteListRequest = ListRequest<SiteAttributes>

const list = async (params?: SiteListRequest) =>
  axios.get<ObjectListResult<SiteAttributes>>('/api/sites', { params })

const view = async (params: { id: string }) =>
  axios.get<SiteAttributes>(`/api/sites/${params.id}`)

const create = async (params: SiteRequest) =>
  axios.post<SiteRequest>('/api/sites', { site: params })

const update = async (id: string, params: SiteRequest) =>
  axios.put<SiteRequest>(`/api/sites/${id}`, { site: params })

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/sites/${params.id}`)

export default {
  list,
  view,
  create,
  update,
  destroy,
}
