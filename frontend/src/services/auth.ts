import axios from 'axios'
import store from '../store'

export interface AuthReadyResponse {
  ready: boolean
  strategy: 'local' | 'oauth'
}

export interface LocalAuthLoginRequest {
  user: {
    login: string
    password: string
  }
}

const logout = async () =>
  axios.get('/api/auth/logout').then(() => {
    store.commit('clearSession')
  })

const login = async (params: LocalAuthLoginRequest) =>
  axios.post<LocalAuthLoginRequest>('/api/auth/login', params)

const ready = async () => axios.get<AuthReadyResponse>('/api/auth/ready')

export default {
  logout,
  login,
  ready,
}
