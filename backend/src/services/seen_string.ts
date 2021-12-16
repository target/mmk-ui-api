import { cachedView, updateCache, writeLRU } from '../api/crud/cache'
import LRUCache from 'lru-native2'
import { SeenString, SeenStringAttributes } from '../models'

export const cache = new LRUCache<number>({
  maxElements: 10000,
  maxAge: 60000,
  size: 1000,
  maxLoadFactor: 2.0,
})

const cached_view = cachedView(SeenString.tableName, cache)
const cached_write_view = updateCache(SeenString.tableName, writeLRU(cache))

const view = async (id: string): Promise<SeenString> =>
  SeenString.query().findById(id).skipUndefined().throwIfNotFound()

const update = async (
  id: string,
  attrs: Partial<SeenStringAttributes>
): Promise<SeenString> => SeenString.query().patchAndFetchById(id, attrs)

const findOne = async (
  query: Partial<SeenStringAttributes>
): Promise<SeenString> => SeenString.query().findOne(query)

const create = async (
  attrs: Partial<SeenStringAttributes>
): Promise<SeenString> => SeenString.query().insert(attrs)

const destroy = async (id: string): Promise<number> =>
  SeenString.query().deleteById(id)

export default {
  view,
  cached_view,
  cached_write_view,
  update,
  findOne,
  create,
  destroy,
}
