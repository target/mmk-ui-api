/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'

export type SecretTypes = 'qt' | 'manual'

export interface SecretAttributes {
  id?: string
  name: string
  type: SecretTypes
  value: string
  created_at?: Date
  updated_at?: Date
}

export interface SecretTypesResponse {
  types: Partial<SecretTypes>[]
}

type SecretCreateRequest = Pick<SecretAttributes, 'name' | 'type' | 'value'>

type SecretUpdateRequest = Pick<SecretAttributes, 'type' | 'value'>

interface SecretListRequest extends ListRequest<SecretAttributes> {
  name?: string
  eager?: ['sources']
}

const list = async (params?: SecretListRequest) =>
  axios.get<ObjectListResult<SecretAttributes>>('/api/secrets', { params })

const view = async (params: { id: string }) =>
  axios.get<SecretAttributes>(`/api/secrets/${params.id}`)

const create = async (params: SecretCreateRequest) =>
  axios.post<SecretCreateRequest>('/api/secrets', { secret: params })

const update = async (id: string, params: SecretUpdateRequest) =>
  axios.put<SecretUpdateRequest>(`/api/secrets/${id}`, { secret: params })

const types = async () => axios.get<SecretTypesResponse>('/api/secrets/types')

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/secrets/${params.id}`)

export default {
  list,
  view,
  create,
  update,
  types,
  destroy,
}
