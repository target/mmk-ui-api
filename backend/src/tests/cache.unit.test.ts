// ./api/crud/cache.ts test
import LRUCache from 'lru-native2'
import { resetDB } from './utils'
import { cachedView, updateCache, writeLRU } from '../api/crud/cache'
import { AllowList } from '../models'
import { redisClient } from '../repos/redis'

const cache = new LRUCache<number>({
  maxElements: 1,
  maxAge: 600,
  size: 1,
  maxLoadFactor: 2.0,
})

const cache_key = 'allow_list:fqdn:example.com'
const cacheQuery = {
  key: 'example.com',
  type: 'fqdn',
  field: 'key',
}
const cached_view = cachedView(AllowList.tableName, cache)
const write_cache_view = updateCache(AllowList.tableName, writeLRU(cache))

describe('CRUD Cache', () => {
  beforeEach(async () => {
    await resetDB()
    await redisClient.del(cache_key)
    cache.clear()
  })
  describe('cachedView', () => {
    it('returns false/none on cache miss', async () => {
      const res = await cached_view(cacheQuery)
      expect(res).toEqual({ has: false, store: 'none' })
    })
    it('returns true/local when found in LRU cache', async () => {
      cache.set(cache_key, 1)
      const res = await cached_view(cacheQuery)
      expect(res).toEqual({ has: true, store: 'local' })
    })
    it('returns true/redis when found in redis cache', async () => {
      await redisClient.set(cache_key, 1)
      const res = await cached_view(cacheQuery)
      expect(res).toEqual({ has: true, store: 'redis' })
    })
  })
  describe('writeRedis', () => {
    it('should update lruCache for redis', async () => {
      let res = cache.get(cache_key)
      expect(res).toBe(undefined)
      await write_cache_view(cacheQuery, 'redis')
      res = cache.get(cache_key)
      expect(res).toBe(1)
    })
    it('should update redis cache and lruCache for database', async () => {
      const res = await redisClient.get(cache_key)
      expect(res).toBe(null)
      await write_cache_view(cacheQuery, 'database')
      const lruRes = cache.get(cache_key)
      expect(lruRes).toBe(1)
      const redisRes = await redisClient.get(cache_key)
      expect(redisRes).toBe('1')
    })
  })
})
