import { Router } from 'express'
import { Path, PathItem, Route } from 'aejo'
import { AuthScope, OauthScope } from '../../middleware/auth'

import localLoginRoute from './local-login'
import logoutRoute from './logout'
import getOauthRoute from './get-oauth'
import oauthCallBackRoute from './oauth-callback'
import readyRoute from './ready'
import sessionRoute from './session'

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/login', localLoginRoute),
    Path('/logout', AuthScope(logoutRoute)),
    Path('/oauth', getOauthRoute),
    Path('/oauth_callback', OauthScope(oauthCallBackRoute)),
    Path('/ready', readyRoute),
    Path('/session', sessionRoute)
  )
