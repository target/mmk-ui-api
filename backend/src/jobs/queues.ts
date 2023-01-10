import Queue from 'bull'
import EventEmitter from 'events'
import { Redis } from 'ioredis'
import MerryMaker from '@merrymaker/types'
import { createClient } from '../repos/redis'

EventEmitter.defaultMaxListeners = 14

const redisClient = createClient()
const redisSubscriber = createClient()

const openClients: Redis[] = [redisClient, redisSubscriber]

const resolveClient = (type: 'client' | 'subscriber' | 'bclient') => {
  if (type === 'client') {
    return redisClient
  } else if (type === 'subscriber') {
    return redisSubscriber
  } else {
    const c = createClient()
    openClients.push(c)
    return c
  }
}

const scannerScheduler = new Queue('scanner-scheduler', {
  createClient: resolveClient
})

const scannerQueue = new Queue<MerryMaker.ScanQueueJob>('scanner-queue', {
  createClient: resolveClient
})

const scannerEventQueue = new Queue('scan-log-queue', {
  createClient
})

const localQueue = new Queue('local', {
  createClient: resolveClient
})

const qtSecretRefresh = new Queue('qt-secret-refresh', {
  createClient: resolveClient
})

const alertQueue = new Queue<MerryMaker.EventResult>('alert-queue', {
  createClient: resolveClient
})

export default {
  localQueue,
  scannerScheduler,
  scannerQueue,
  scannerEventQueue,
  qtSecretRefresh,
  alertQueue
}
