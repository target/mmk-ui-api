/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'

export type AllowListType =
  | 'fqdn'
  | 'ip'
  | 'ioc-payload-domain'
  | 'literal'
  | 'google-analytics'

export interface AllowListAttributes {
  id: string
  type: AllowListType
  key: string
  created_at: Date
}

export interface AllowListRequest {
  id?: string
  type: AllowListType
  key: string
}

interface AllowListListRequest extends ListRequest<AllowListAttributes> {
  search?: string
  type?: AllowListType
}

const list = async (params?: AllowListListRequest) =>
  axios.get<ObjectListResult<AllowListAttributes>>('/api/allow_list', {
    params,
  })

const view = async (params: { id: string }) =>
  axios.get<AllowListAttributes>(`/api/allow_list/${params.id}`)

const create = async (params?: AllowListRequest) =>
  axios.post<AllowListRequest>('/api/allow_list', { allow_list: params })

const update = async (id: string, params: AllowListRequest) =>
  axios.put<AllowListRequest>(`/api/allow_list/${id}`, {
    allow_list: params,
  })

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/allow_list/${params.id}`)

export default {
  list,
  view,
  create,
  update,
  destroy,
}
