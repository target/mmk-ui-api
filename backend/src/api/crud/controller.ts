/* eslint-disable @typescript-eslint/no-explicit-any */
import { NextFunction, Request, Response, Router } from 'express'
import Ajv from 'ajv'
import { IJsonSchema, OpenAPIV3 } from 'openapi-types'
import { AccessControl } from 'role-acl'
import {
  ForbiddenError,
  UnauthorizedError,
  ClientError,
} from '../middleware/client-errors'
import { UserRole } from '../../models/users'

export type AccessControlContext<T> = (req: Request, res: Response) => T

export interface RouteAccessControl<T> {
  grant: UserRole
  condition?: <T>(content: T) => boolean
  context?: AccessControlContext<T>
}

export type RouteFunction = (
  req: Request,
  res: Response,
  next: NextFunction
) => Promise<void> | void

const ajv = new Ajv({ coerceTypes: true })

export interface RouteOptions {
  doc?: OpenAPIV3.OperationObject
  path: string
  before?: RouteFunction[] | RouteFunction
  validate?: {
    req: 'query' | 'body' | 'params'
    schema: IJsonSchema
  }
  handler: RouteFunction[] | RouteFunction
  method: 'get' | 'post' | 'put' | 'delete'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  accessControl: RouteAccessControl<any>
}

export const paramAsAjv = (
  params: OpenAPIV3.PathsObject[]
): { [name: string]: IJsonSchema } => {
  const properties: Record<string, any> = {}
  params.forEach((p) => {
    properties[p.name as string] = p.schema
  })
  return properties
}

function spreadArray(arr: RouteFunction | RouteFunction[]): RouteFunction[] {
  if (!Array.isArray(arr)) {
    return [arr]
  }
  return arr
}

export interface CrudOptions {
  app: Router
  ac: AccessControl
  beforeAll?: RouteFunction[] | RouteFunction
  route: string
  routes: Record<string, RouteOptions>
}

export function exceptionWrapper(
  handlerFunc: RouteFunction[]
): RouteFunction[] {
  return handlerFunc.map(
    (h) => async (req: Request, res: Response, next: NextFunction) => {
      try {
        await h(req, res, next)
      } catch (e) {
        next(e)
      }
    }
  )
}

function accessControlBuilder(ac: AccessControl) {
  return function (r: RouteOptions, fullPath: string) {
    ac.grant(r.accessControl.grant)
      .condition(r.accessControl.condition)
      .execute(r.method)
      .on(fullPath)
    return async (req: Request, _res: Response, next: NextFunction) => {
      if (req.session === undefined || req.session.data === undefined) {
        next(
          new UnauthorizedError(
            'not authorized to access this resource',
            'guest'
          )
        )
        return
      }
      const access = await ac
        .can(req.session.data.role)
        .context(r.accessControl.context)
        .execute(r.method)
        .sync()
        .on(fullPath)
      if (access.granted) {
        next()
      } else {
        next(new ForbiddenError(`${r.method} ${fullPath}`, access))
      }
    }
  }
}

function validateBuilder(r: RouteOptions) {
  const validate = ajv.compile(r.validate.schema)
  return (req: Request, _res: Response, next: NextFunction) => {
    if (!validate(req[r.validate.req])) {
      throw new ClientError('invalid query', {
        type: 'client',
        event: validate.errors,
      })
    }
    next()
  }
}

export default function controller(opts: CrudOptions): OpenAPIV3.PathsObject {
  const route = Router()

  opts.app.use(opts.route, route)

  if (opts.beforeAll) {
    route.use(exceptionWrapper(spreadArray(opts.beforeAll)))
  }

  const acBuilder = accessControlBuilder(opts.ac)
  const docPaths: OpenAPIV3.PathsObject = {}
  Object.values(opts.routes).forEach((r) => {
    const fullPath = `${opts.route}${r.path}`
    const paths: RouteFunction[] = []
    if (r.before) {
      paths.push(...spreadArray(r.before))
    }
    if (r.validate) {
      paths.push(validateBuilder(r))
    }
    paths.push(acBuilder(r, fullPath))
    paths.push(...spreadArray(r.handler))
    route[r.method](r.path, exceptionWrapper(paths))
    if (r.doc) {
      if (docPaths.fullPath !== undefined) {
        docPaths[fullPath][r.method] = r.doc
      } else {
        docPaths[fullPath] = {
          [r.method]: r.doc,
        }
      }
    }
  })
  return docPaths
}
