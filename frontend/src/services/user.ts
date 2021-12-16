/* eslint-disable camelcase */
import axios from 'axios'
import { ObjectListResult, ListRequest } from './index'

export type UserRole = 'admin' | 'user' | 'transport' | 'guest'

export interface UserAttributes {
  id?: string
  login: string
  password?: string
  role: UserRole
  created_at?: Date
  updated_at?: Date
}

interface UserListRequest extends ListRequest<UserAttributes> {
  role?: string
  login?: string
}

const view = async (params: { id: string }) =>
  axios.get<UserAttributes>(`/api/users/${params.id}`)

const list = async (params?: UserListRequest) =>
  axios.get<ObjectListResult<UserAttributes>>('/api/users', { params })

const create = async (params?: {
  user: Pick<UserAttributes, 'login' | 'role' | 'password'>
}) => axios.post<UserAttributes>('/api/users', params)

const createAdmin = async (params?: Pick<UserAttributes, 'password'>) =>
  axios.post<UserAttributes>('/api/users/create_admin', params)

const update = async (params: {
  id: string
  user: Pick<UserAttributes, 'login' | 'password' | 'role'>
}) => axios.put<UserAttributes>(`/api/users/${params.id}`, params)

const destroy = async (params: { id: string }) =>
  axios.delete(`/api/users/${params.id}`)

export default {
  view,
  list,
  create,
  createAdmin,
  update,
  destroy,
}
