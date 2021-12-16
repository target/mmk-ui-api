import { AuthPathOp, Path, PathItem, Route, Scope } from 'aejo'
import { Router } from 'express'
import { Authorized } from '../../middleware/auth'
import listRoute from './list'
import createRoute from './create'
import viewRoute from './view'
import createAdminRoute from './create-admin'
import updateRoute from './update'
import deleteRoute from './delete'
import { uuidFormat } from '../../crud/schemas'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AdminScope(listRoute), AdminScope(createRoute)),
    Path('/create_admin', createAdminRoute),
    Path(
      `/:id(${uuidFormat})`,
      AdminScope(updateRoute),
      AdminScope(viewRoute),
      AdminScope(deleteRoute)
    )
  )
