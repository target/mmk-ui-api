import { AuthPathOp, Path, PathItem, Route, Scope } from 'aejo'
import { Router } from 'express'
import { uuidFormat } from '../..//crud/schemas'
import { AuthScope, Authorized } from '../../middleware/auth'

import listRoute from './list'
import viewRoute from './view'
import deleteRoute from './delete'
import bulkDeleteRoute from './bulk-delete'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AuthScope(listRoute)),
    Path(`/:id(${uuidFormat})`, AuthScope(viewRoute), AdminScope(deleteRoute)),
    Path('/bulk_delete', AdminScope(bulkDeleteRoute))
  )
