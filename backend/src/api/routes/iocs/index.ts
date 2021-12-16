import { Router } from 'express'
import listRoute from './list'
import createRoute from './create'
import bulkCreateRoute from './bulk-create'
import cacheViewRoute from './cache-view'
import viewRoute from './view'
import updateRoute from './update'
import deleteRoute from './delete'

import { Authorized, AuthScope } from '../../middleware/auth'
import { AuthPathOp, Scope, Route, PathItem, Path } from 'aejo'
import { uuidFormat } from '../../crud/schemas'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))
const TransportScope = AuthPathOp(Scope(Authorized, 'transport'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AuthScope(listRoute), AdminScope(createRoute)),
    Path('/bulk', AdminScope(bulkCreateRoute)),
    Path('/_cache', TransportScope(cacheViewRoute)),
    Path(
      `/:id(${uuidFormat})`,
      AuthScope(viewRoute),
      AdminScope(updateRoute),
      AdminScope(deleteRoute)
    )
  )
