import Vue from 'vue'
import axios from 'axios'
import Vuex from 'vuex'

Vue.use(Vuex)

export interface Session {
  role: 'admin' | 'transport' | 'user'
  firstName: string
  lastName: string
  email: string
  exp: number
  lanid: string
  isAuth: boolean
}

export interface Notifications {
  type: 'error' | 'info' | 'warning'
  title: string
  body: string
}

export default new Vuex.Store({
  state: {
    user: {} as Session | undefined,
    notifications: {} as Notifications,
    barColor: 'rgba(0, 0, 0, .8), rgba(0, 0, 0, .8)',
    barImage: require('@/assets/sky.webp'),
    drawer: null,
  },
  mutations: {
    clearSession(state) {
      state.user = {} as Session
    },
    setSession(state, session: Session) {
      state.user = session
    },
    setBarImage(state, payload) {
      state.barImage = payload
    },
    setDrawer(state, payload) {
      state.drawer = payload
    },
    setNotifications(state, payload: Notifications) {
      state.notifications = payload
    },
    clearNotifications(state) {
      state.notifications = {} as Notifications
    },
  },
  actions: {
    getSession({ commit }) {
      return new Promise((resolve, reject) => {
        axios({ url: '/api/auth/session', method: 'GET' })
          .then((resp) => {
            commit('setSession', resp.data)
            resolve(resp.data)
          })
          .catch((err) => {
            if (err.response) {
              // flash error
            }
            commit('clearSession')
            reject(err.message)
          })
      })
    },
  },
  modules: {},
  getters: {
    isLoggedIn: (state) => state.user?.isAuth === true,
    user: (state) => state.user,
    notifications: (state) => state.notifications,
  },
})
