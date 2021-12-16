// api/routes/allow_list.ts
import { Path, PathItem, Route, AuthPathOp, Scope } from 'aejo'
import { Router } from 'express'
import { Authorized, AuthScope } from '../../middleware/auth'
import { uuidFormat } from '../../crud/schemas'

import listRoute from './list'
import createRoute from './create'
import cacheRoute from './cache'
import viewRoute from './view'
import updateRoute from './update'
import deleteRoute from './delete'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))
const TransportScope = AuthPathOp(Scope(Authorized, 'transport'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path(
      '/',
      // All auth users
      AuthScope(listRoute),
      // Admin users
      AuthPathOp(Scope(Authorized, 'admin', 'transport'))(createRoute)
    ),
    Path('/_cache', TransportScope(cacheRoute)),
    Path(
      `/:id(${uuidFormat})`,
      AuthScope(viewRoute),
      AdminScope(updateRoute),
      AdminScope(deleteRoute)
    )
  )
