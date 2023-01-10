import Redis, { RedisOptions } from 'ioredis'
import { config, } from 'node-config-ts'

const defaultOpts: RedisOptions = {
  maxRetriesPerRequest: null,
  enableReadyCheck: false,
}

function createClient(): Redis {
  if (
    config.redis?.useSentinel &&
    config.redis.nodes &&
    Array.isArray(config.redis.nodes)
  ) {
    const clients = config.redis.nodes.map((item: string) => ({
      host: item.trim(),
      port: config.redis.sentinelPort
    }))
    return new Redis({
      updateSentinels: false,
      sentinels: clients,
      name: config.redis.master,
      password: config.redis.sentinelPassword,
      sentinelPassword: config.redis.sentinelPassword,
      ...defaultOpts
    })
  }
  return new Redis(config.redis.uri, defaultOpts)
}

const redisClient = createClient()
export { createClient, redisClient }
