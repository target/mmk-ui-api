export interface ObjectListResult<M> {
  results: M[]
  total: number
}

export type ObjectDistinctResult<M> = Array<Record<keyof M, string>>

export interface ListRequest<M> {
  fields?: Array<keyof M>
  page?: number
  pageSize?: number
  orderColumn?: keyof M
  orderDirection?: 'asc' | 'desc'
}
