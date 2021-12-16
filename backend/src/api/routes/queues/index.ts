import { Router } from 'express'
import { Route, Path, PathItem, Scope, AuthPathOp } from 'aejo'
import { Authorized } from '../../middleware/auth'

import jobsRoute from './jobs'

const UserScope = AuthPathOp(Scope(Authorized, 'user'))
export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(router, Path('/', UserScope(jobsRoute)))
