import Queue from 'bull'
import { redisClient, createClient } from '../repos/redis'

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

let scannerQueue: Queue.Queue

// Create new or use existing instance
export async function getScannerQueue(): Promise<Queue.Queue> {
  if (!scannerQueue) {
    scannerQueue = new Queue('scanner-queue', { createClient: resolveClient })
  }
  await scannerQueue.isReady()
  return scannerQueue
}
