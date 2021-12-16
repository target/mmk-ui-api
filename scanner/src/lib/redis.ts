import redis from 'ioredis'

import { config } from 'node-config-ts'

function createClient(): redis.Redis {
  if (config.redis.useSentinel) {
    const clients = config.redis.nodes.map((item: string) => ({
      host: item.trim(),
      port: config.redis.sentinelPort,
    }))
    return new redis({
      updateSentinels: false,
      sentinels: clients,
      name: config.redis.master,
      password: config.redis.sentinelPassword,
    })
  }
  return new redis(config.redis.uri)
}

export const client = createClient()
export const subscriber = createClient()

export function resolveClient(type: string): redis.Redis {
  switch (type) {
    case 'client':
      return client
    case 'subscriber':
      return subscriber
    default:
      return createClient()
  }
}
