import Queue from 'bull'
import MerryMaker from '@merrymaker/types'
import { createClient } from '../repos/redis'

const redisClient = createClient()
const redisSubscriber = createClient()

const resolveClient = (type: string) => {
  if (type === 'client') {
    return redisClient
  } else if (type === 'subscriber') {
    return redisSubscriber
  } else {
    return createClient()
  }
}

const scannerScheduler = new Queue('scanner-scheduler', {
  prefix: 'mmk',
  createClient: resolveClient,
})

const scannerQueue = new Queue<MerryMaker.ScanQueueJob>('scanner-queue', {
  createClient,
})

const scannerEventQueue = new Queue('scan-log-queue', {
  createClient,
})

const scannerPurge = new Queue('scanner-purge', {
  createClient,
})

const qtSecretRefresh = new Queue('qt-secret-refresh', {
  createClient,
})

const alertQueue = new Queue<MerryMaker.EventResult>('alert-queue', {
  createClient,
})

export default {
  scannerScheduler,
  scannerQueue,
  scannerEventQueue,
  scannerPurge,
  qtSecretRefresh,
  alertQueue,
}
