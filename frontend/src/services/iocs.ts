/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'

export type IocType = 'fqdn' | 'ip' | 'regex' | 'wildcard' | 'literal'

export interface IocAttributes {
  id: string
  type: IocType
  value: string
  enabled: boolean
  created_at: Date
}

export interface IocRequest {
  ioc: {
    id?: string
    type: IocType
    value: string
    enabled: boolean
  }
}

export interface IocBulkCreateRequest {
  iocs: {
    values: string[]
    enabled: boolean
    type: IocType
  }
}

interface IocListRequest extends ListRequest<IocAttributes> {
  enabled?: boolean
  search?: string
  type?: IocType
}

const list = async (params?: IocListRequest) =>
  axios.get<ObjectListResult<IocAttributes>>('/api/iocs', { params })

const view = async (params: { id: string }) =>
  axios.get<IocAttributes>(`/api/iocs/${params.id}`)

const create = async (params?: IocRequest) =>
  axios.post<IocRequest>('/api/iocs', params)

const bulkCreate = async (params?: IocBulkCreateRequest) =>
  axios.post<IocBulkCreateRequest>('/api/iocs/bulk', params)

const update = async (id: string, params: IocRequest) =>
  axios.put<IocRequest>(`/api/iocs/${id}`, params)

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/iocs/${params.id}`)

export default {
  list,
  view,
  create,
  bulkCreate,
  update,
  destroy,
}
