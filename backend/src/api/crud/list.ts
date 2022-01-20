import { Request, Response, NextFunction } from 'express'
import { OpenAPIV3 } from 'openapi-types'
import { OrderByDirection, Model } from 'objection'
import { BaseClass } from '../../models/base'
import { ParamSchema, Integer, QueryParam, Parameter } from 'aejo'

export interface ListRequest {
  page?: number
  pageSize?: number
  orderColumn: string
  orderDirection: OrderByDirection
}

export const ListQueryParams: Parameter[] = [
  QueryParam({
    name: 'pageSize',
    description: 'max number per page',
    schema: Integer({ minimum: -1 }),
  }),
  QueryParam({
    name: 'page',
    description: 'page number',
    schema: Integer({ minimum: 1 }),
  }),
  QueryParam({
    name: 'orderColumn',
    description: 'order by column',
    schema: {
      type: 'string',
    },
  }),
  QueryParam({
    name: 'orderDirection',
    description: 'order direction',
    schema: {
      type: 'string',
      enum: ['asc', 'desc'],
    },
  }),
]

export const listResponseSchema = (schema: {
  [p: string]: ParamSchema
}): Record<string, ParamSchema> => ({
  total: {
    type: 'integer',
    description: 'total number of results',
  },
  results: {
    type: 'array',
    items: {
      type: 'object',
      properties: schema,
    },
  },
})

export const listHandlerParams = (
  sort: string[]
): OpenAPIV3.ParameterObject[] => [
  {
    in: 'query',
    name: 'pageSize',
    schema: {
      type: 'integer',
      default: 20,
      minimum: 0,
      maximum: 1000,
    },
    description: 'Number of results per page',
    example: {
      large: {
        value: 100,
        summary: 'Return 100 results per page',
      },
    },
  },
  {
    in: 'query',
    name: 'page',
    schema: {
      type: 'integer',
      default: 1,
      minimum: 0,
    },
    description: 'Page number from 1',
  },
  {
    in: 'query',
    name: 'orderColumn',
    schema: {
      type: 'string',
      enum: sort,
    },
    description: 'Sort by field',
  },
  {
    in: 'query',
    name: 'orderDirection',
    schema: {
      type: 'string',
      enum: ['asc', 'desc'],
      default: 'desc',
    },
    description: 'Sort direction',
  },
]

function getPagable(
  req: { page?: number | string; pageSize?: number | string },
  defaultSize = 20
) {
  let page = 1
  let pageSize = defaultSize
  if (req.page !== undefined) {
    if (typeof req.page === 'string') {
      page = parseInt(req.page, 10)
    } else {
      page = req.page
    }
  }
  if (req.pageSize !== undefined) {
    if (typeof req.pageSize === 'string') {
      pageSize = parseInt(req.pageSize, 10)
    } else {
      pageSize = req.pageSize
    }
  }
  return { page, pageSize }
}

function getOrder(
  req: { orderColumn?: string; orderDirection?: string },
  selectable: (string | number | symbol)[],
  defaultOrder = 'created_at'
) {
  let orderColumn = defaultOrder
  let orderDirection: OrderByDirection
  if (req.orderColumn && selectable.includes(req.orderColumn)) {
    orderColumn = req.orderColumn
  }
  if (
    req.orderDirection &&
    ['asc', 'desc'].includes(req.orderDirection.toLowerCase())
  ) {
    orderDirection = req.orderDirection as OrderByDirection
  }
  return { orderColumn, orderDirection }
}

export function listHandler<M extends Model>(
  model: BaseClass<M>,
  selectable?: string[]
) {
  return async (
    req: Request,
    res: Response,
    next: NextFunction
  ): Promise<void> => {
    const { page, pageSize } = getPagable(req.query)
    const { orderColumn, orderDirection } = getOrder(req.query, selectable)
    let fields = selectable
    if (res.locals.selectable && Array.isArray(res.locals.selectable)) {
      fields = res.locals.selectable.filter((s: string) =>
        selectable.includes(s)
      )
    }
    const results = await model
      .query()
      .select(fields)
      .skipUndefined()
      .modify(res.locals.whereBuilder)
      .page(page - 1, pageSize <= 0 ? undefined : pageSize)
      .orderBy(orderColumn, orderDirection)
    res.status(200).send(results)
    next()
  }
}
