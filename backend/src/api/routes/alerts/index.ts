// api/routes/alerts.ts
import { Router } from 'express'
import { AuthPathOp, Path, PathItem, Route, Scope } from 'aejo'
import { Authorized, AuthScope } from '../../middleware/auth'
import { uuidFormat } from '../../crud/schemas'

import listRoute from './list'
import viewRoute from './view'
import deleteRoute from './delete'
import distinctRoute from './distinct'
import aggRoute from './agg'

const AdminScope = AuthPathOp(Scope(Authorized, 'admin'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AuthScope(listRoute)),
    Path('/agg', AuthScope(aggRoute)),
    Path(`/:id(${uuidFormat})`, AuthScope(viewRoute), AdminScope(deleteRoute)),
    Path('/distinct', AuthScope(distinctRoute))
  )
