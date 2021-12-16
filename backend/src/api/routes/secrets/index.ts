import { Router } from 'express'
import { AuthPathOp, Scope, PathItem, Path, Route } from 'aejo'
import { Authorized } from '../../middleware/auth'

import { uuidFormat } from '../../crud/schemas'

import listRoute from './list'
import createRoute from './create'
import updateRoute from './update'
import getTypesRoute from './get-types'
import viewRoute from './view'
import deleteRoute from './delete'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AdminScope(listRoute), AdminScope(createRoute)),
    Path(
      `/:id(${uuidFormat})`,
      AdminScope(viewRoute),
      AdminScope(updateRoute),
      AdminScope(deleteRoute)
    ),
    Path('/types', AdminScope(getTypesRoute))
  )
