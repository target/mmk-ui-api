export interface ObjectListResult<M> {
  results: M[]
  total: number
}

export interface ListRequest<M> {
  fields?: Array<keyof M>
  page?: number
  pageSize?: number
  orderColumn?: keyof M
  orderDirection?: 'asc' | 'desc'
}
