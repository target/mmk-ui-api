import { Router } from 'express'
import { Route, PathItem, Path, AuthPathOp, Scope } from 'aejo'
import { Authorized } from '../../middleware/auth'
import { uuidFormat } from '../../crud/schemas'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))
const UserScope = AuthPathOp(Scope(Authorized, 'user'))

import listRoute from './list'
import createRoute from './create'
import updateRoute from './update'
import viewRoute from './view'
import deleteRoute from './delete'

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', UserScope(listRoute), AdminScope(createRoute)),
    Path(
      `/:id(${uuidFormat})`,
      UserScope(viewRoute),
      AdminScope(updateRoute),
      AdminScope(deleteRoute)
    )
  )
