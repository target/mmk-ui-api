import redis from 'ioredis'
import { config } from 'node-config-ts'

function createClient(): redis.Redis {
  if (config.redis?.useSentinel && config.redis.nodes) {
    const clients = config.redis.nodes.map((item: string) => ({
      host: item.trim(),
      port: config.redis.sentinelPort,
    }))
    return new redis({
      updateSentinels: false,
      sentinels: clients,
      name: config.redis.master,
      password: config.redis.sentinelPassword,
      sentinelPassword: config.redis.sentinelPassword,
    })
  }
  return new redis(config.redis.uri)
}

const redisClient = createClient()
export { createClient, redisClient }
