import { cachedView } from '../api/crud/cache'
import LRUCache from 'lru-native2'
import { AllowList, AllowListAttributes } from '../models'

export const cache = new LRUCache<number>({
  maxElements: 10000,
  maxAge: 60000,
  size: 1000,
  maxLoadFactor: 2.0,
})

const cached_view = cachedView(AllowList.tableName, cache)

const view = async (id: string): Promise<AllowList> =>
  AllowList.query().findById(id).throwIfNotFound()

const update = async (
  id: string,
  attrs: Partial<AllowListAttributes>
): Promise<AllowList> => AllowList.query().patchAndFetchById(id, attrs)

const findOne = async (
  query: Partial<AllowListAttributes>
): Promise<AllowList> => AllowList.query().findOne(query)

const create = async (
  attrs: Partial<AllowListAttributes>
): Promise<AllowList> => AllowList.query().insert(attrs)

const destroy = async (id: string): Promise<number> =>
  AllowList.query().deleteById(id)

export default {
  view,
  findOne,
  cached_view,
  create,
  update,
  destroy,
}
