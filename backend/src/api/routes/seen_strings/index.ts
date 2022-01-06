import { Router } from 'express'
import { AuthPathOp, Path, PathItem, Route, Scope } from 'aejo'
import { Authorized, AuthScope } from '../../middleware/auth'
import { uuidFormat } from '../../crud/schemas'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))
const TransportScope = AuthPathOp(Scope(Authorized, 'transport'))

import listRoute from './list'
import viewRoute from './view'
import distinctRoute from './distinct'
import updateRoute from './update'
import deleteRoute from './delete'
import createRoute from './create'
import getCacheRoute from './get-cache'
import cacheRoute from './cache'

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AdminScope(listRoute), AdminScope(createRoute)),
    Path('/distinct', AuthScope(distinctRoute)),
    Path('/_cache', TransportScope(cacheRoute), TransportScope(getCacheRoute)),
    Path(
      `/:id(${uuidFormat})`,
      AdminScope(viewRoute),
      AdminScope(updateRoute),
      AdminScope(deleteRoute)
    )
  )
