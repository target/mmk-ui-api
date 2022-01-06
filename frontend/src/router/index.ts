import Vue from 'vue'
import VueRouter, { RouteConfig } from 'vue-router'
import store from '../store'

Vue.use(VueRouter)

const uuidFormat =
  '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'

const routes: Array<RouteConfig> = [
  {
    path: '/login',
    name: 'Login',
    component: () =>
      import(/* webpackChunkName: "login" */ '../views/Login.vue'),
    meta: {
      forwardAuth: true
    }
  },
  {
    path: '/',
    component: () => import('../views/dashboard/Index.vue'),
    meta: {
      authorize: ['user']
    },
    children: [
      {
        name: 'Dashboard',
        path: '',
        component: () => import('../views/dashboard/Dashboard.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'Alerts',
        path: '/alerts',
        component: () => import('../views/dashboard/Alerts.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'Sources',
        path: '/sources',
        component: () => import('../views/dashboard/Sources.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'Seen Strings',
        path: '/seen_strings',
        component: () => import('../views/dashboard/SeenStrings.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'SeenStringForm',
        path: `/seen_strings/edit`,
        component: () => import('../views/seen_strings/SeenStringForm.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'Scans',
        path: '/scans',
        component: () => import('../views/dashboard/Scans.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'Sites',
        path: '/sites',
        component: () => import('../views/dashboard/Sites.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'Secrets',
        path: '/secrets',
        component: () => import('../views/dashboard/Secrets.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'SecretsForm',
        path: '/secrets/:id',
        component: () => import('../views/secrets/SecretForm.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'ScanLog',
        path: `/scans/:id(${uuidFormat})`,
        component: () => import('../views/scans/ScanLog.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'Site',
        path: `/site/:id(${uuidFormat})`,
        component: () => import('../views/sites/Site.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'SiteForm',
        path: '/site/edit/:id',
        component: () => import('../views/sites/SiteForm.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'SourceForm',
        path: '/source/edit',
        component: () => import('../views/sources/SourceForm.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'IocForm',
        path: '/ioc/edit',
        component: () => import('../views/iocs/IocForm.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'IOCs',
        path: '/iocs',
        component: () => import('../views/dashboard/IOCs.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'AllowListForm',
        path: '/allow_list/edit',
        component: () => import('../views/allow_list/AllowListForm.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'AllowList',
        path: '/allow_list',
        component: () => import('../views/dashboard/AllowList.vue'),
        meta: {
          authorize: ['user']
        }
      },
      {
        name: 'Users',
        path: '/users',
        component: () => import('../views/dashboard/Users.vue'),
        meta: {
          authorize: ['admin']
        }
      },
      {
        name: 'UserForm',
        path: '/users/edit',
        component: () => import('../views/users/UserForm.vue'),
        meta: {
          authorize: ['admin']
        }
      }
    ]
  }
]

const router = new VueRouter({
  mode: 'history',
  base: process.env.BASE_URL,
  routes
})

router.beforeEach(async (to, _from, next) => {
  let authorize
  let forwardAuth

  if (to.meta !== undefined) {
    ({ authorize, forwardAuth } = to.meta)
  }

  if (authorize) {
    if (!store.getters.isLoggedIn) {
      try {
        await store.dispatch('getSession')
      } catch (e) {
        return next('/login')
      }
    }

    if (store.getters.user.role === 'admin') {
      return next()
    }

    if (authorize.length && authorize.includes(store.getters.user.role)) {
      return next()
    } else {
      setTimeout(() => {
        store.commit('setNotifications', {
          type: 'warning',
          title: 'Not authorized',
          body: 'Not authorized to access this resource'
        })
      }, 100)
      return next('/')
    }
  } else if (forwardAuth) {
    if (!store.getters.isLoggedIn) {
      try {
        await store.dispatch('getSession')
        return next('/')
      } catch (e) {}
    }
    return next()
  } else {
    next('/')
    console.error('no meta', to, authorize, forwardAuth)
  }
})

export default router
