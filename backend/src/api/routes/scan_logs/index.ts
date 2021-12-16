import { AuthPathOp, Path, PathItem, Route, Scope } from 'aejo'
import { Router } from 'express'
import { uuidFormat } from '../..//crud/schemas'
import { Authenticated } from '../../middleware/auth'

import listRoute from './list'
import viewRoute from './view'
import distinctRoute from './distinct'

const AuthScope = AuthPathOp(Scope(Authenticated, 'user'))

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path('/', AuthScope(listRoute)),
    Path(`/:id(${uuidFormat})`, AuthScope(viewRoute)),
    Path(`/:id(${uuidFormat})/distinct`, AuthScope(distinctRoute))
  )
