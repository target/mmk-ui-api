import { Request, Response, NextFunction } from 'express'
import { UserRole } from '../../models/users'
import AuthService from '../../services/auth'
import { UnauthorizedError, ForbiddenError } from '../middleware/client-errors'

// TODO - move to global
export type routeFunction = (
  req: Request,
  res: Response,
  next: NextFunction
) => Promise<void> | void

export class ControllerUtil {
  constructor(private name: string) {}
  /**
   * isAuth
   *  middleware check to ensure client is authenticated
   */
  public isAuth = () => {
    return (req: Request, _res: Response, next: NextFunction): void => {
      if (!AuthService.isAuth(req.session)) {
        throw new UnauthorizedError(this.name, 'unauthenticated')
      }
      next()
    }
  }

  /**
   * isRole
   *   verifies user role matches named role
   *   throws ForbiddenError if session data does not match
   */
  public isRole = (role: UserRole) => {
    return (req: Request, _res: Response, next: NextFunction): void => {
      if (!AuthService.isRole(req.session.data, role)) {
        throw new ForbiddenError(this.name, 'unauthorized')
      }
      next()
    }
  }

  /**
   * hasRole
   *   verifies user role based on level
   *   throws ForbiddenError if session is below the provided role level
   */
  public hasRole = (role: UserRole) => {
    return (req: Request, _res: Response, next: NextFunction): void => {
      if (
        req.session?.data === undefined ||
        !AuthService.hasRole(req.session.data, role)
      ) {
        throw new ForbiddenError(this.name, 'unauthorized')
      }
      next()
    }
  }

  /**
   * exWrap
   *  Unhandled Exception Wrapper
   */
  public exWrap(asyncFunc: routeFunction[]): routeFunction[] {
    const ret: routeFunction[] = []
    for (const f of asyncFunc) {
      ret.push(async (req: Request, res: Response, next: NextFunction) => {
        try {
          await f(req, res, next)
        } catch (e) {
          next(e)
        }
      })
    }
    return ret
  }
}
