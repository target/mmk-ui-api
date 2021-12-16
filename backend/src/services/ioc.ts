import { Ioc, IocAttributes } from '../models'
import { cachedView } from '../api/crud/cache'
import LRUCache from 'lru-native2'
import { IocType } from '../models/iocs'

export const cache = new LRUCache<number>({
  maxElements: 10000,
  maxAge: 60000,
  size: 1000,
  maxLoadFactor: 2.0,
})

export type IocBulkCreate = {
  values: string[]
  type: IocType
  enabled: boolean
}

const cached_view = cachedView(Ioc.tableName, cache)

const view = async (id: string): Promise<Ioc> =>
  Ioc.query().findById(id).skipUndefined().throwIfNotFound()

const findOne = async (query: Partial<IocAttributes>): Promise<Ioc> =>
  Ioc.query().findOne(query)

const create = async (attrs: Partial<IocAttributes>): Promise<Ioc> =>
  Ioc.query().insert(attrs)

const bulkCreate = async (bulk: IocBulkCreate): Promise<void> => {
  const iocs: IocAttributes[] = bulk.values.map((value) => ({
    value,
    type: bulk.type,
    enabled: bulk.enabled,
  }))
  await Ioc.query().insert(iocs).onConflict(['value', 'type']).ignore()
}

const update = async (
  id: string,
  attrs: Partial<IocAttributes>
): Promise<Ioc> => Ioc.query().patchAndFetchById(id, attrs)

const destroy = async (id: string): Promise<number> =>
  Ioc.query().deleteById(id)

export default {
  view,
  destroy,
  findOne,
  update,
  cached_view,
  create,
  bulkCreate,
}
