/* eslint-disable no-fallthrough */
import LRUCache from 'lru-native2'
import { redisClient } from '../../repos/redis'
import { ParamSchema, QueryParam } from 'aejo'

type CacheStore = 'local' | 'redis' | 'database' | 'none'
export type CacheQueryParam = { key: string; type: string }
export type CacheResponse = { has: boolean; store: CacheStore }
export const cacheViewSchema: ParamSchema = {
  type: 'object',
  properties: {
    has: {
      type: 'boolean',
      description: 'Value was found in cache',
    },
    store: {
      type: 'string',
      description: 'Which store the value was found',
      enum: ['local', 'redis', 'database', 'none'],
    },
  },
}

const typeParam: ParamSchema = {
  type: 'string',
  description: 'Cache type',
}

const keyParam: ParamSchema = {
  type: 'string',
  description: 'Cache key to match on',
}

export const cacheViewParams = [
  QueryParam({
    name: 'type',
    schema: typeParam,
    required: true,
  }),
  QueryParam({
    name: 'key',
    schema: keyParam,
    required: true,
  }),
]

export const updateCacheViewBody = {
  description: 'Cache View',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          cache: {
            type: 'object',
            properties: {
              type: typeParam,
              key: keyParam,
            },
            required: ['type', 'key'],
            additionalProperties: false,
          },
        },
        required: ['cache'],
        additionalProperties: false,
      },
    },
  },
}

export const cachedView = (prefix: string, cache: LRUCache<number>) => async (
  query: CacheQueryParam
): Promise<CacheResponse> => {
  const { type, key } = query
  const cacheKey = `${prefix}:${type}:${key}`
  if (cache.get(cacheKey)) {
    return { has: true, store: 'local' }
  }

  const fromRedis = await redisClient.get(cacheKey)
  if (fromRedis === '1') {
    return { has: true, store: 'redis' }
  }

  return { has: false, store: 'none' }
}

export const writeRedis = async (key: string): Promise<'OK'> =>
  redisClient.set(key, 1, 'EX', 36000)

export const writeLRU = (cache: LRUCache<number>) => (key: string): void =>
  cache.set(key, 1)

export const updateCache = (
  prefix: string,
  lruCache: ReturnType<typeof writeLRU>
) => async (query: CacheQueryParam, store: CacheStore): Promise<CacheStore> => {
  if (store === 'local') {
    return
  }
  const cacheKey = `${prefix}:${query.type}:${query.key}`
  switch (store) {
    default:
    case 'database':
      await writeRedis(cacheKey)
    case 'redis':
      lruCache(cacheKey)
  }
  return store
}
