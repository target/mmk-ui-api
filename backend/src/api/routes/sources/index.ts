import { Router } from 'express'
import { PathItem, Route, Path, AuthPathOp, Scope } from 'aejo'
import { Authorized } from '../../middleware/auth'

import { uuidFormat } from '../../crud/schemas'

import listRoute from './list'
import deleteRoute from './delete'
import viewRoute from './view'
import createRoute from './create'
import createTestRoute from './create-test'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AdminScope(listRoute), AdminScope(createRoute)),
    Path(`/:id(${uuidFormat})`, AdminScope(viewRoute), AdminScope(deleteRoute)),
    Path('/test', AdminScope(createTestRoute))
  )
