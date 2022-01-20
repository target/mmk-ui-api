import { Router, Request, Response, NextFunction } from 'express'
import { Get, Path, PathItem, Route } from 'aejo'

export default (router: Router): { paths: PathItem[]; router: Router } =>
  Route(
    router,
    Path(
      '',
      Get({
        tags: ['health check'],
        description: 'API Health Check',
        middleware: [
          (_req: Request, res: Response, next: NextFunction) => {
            res.status(200).send('ok')
            next()
          }
        ],
        responses: {
          200: {
            description: 'Ok',
            content: {
              'text/plain': {
                schema: {
                  type: 'string',
                  example: 'ok'
                }
              }
            }
          }
        }
      })
    )
  )
