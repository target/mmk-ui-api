/* eslint-disable no-constant-condition */
/* eslint-disable @typescript-eslint/no-explicit-any */
import { EventEmitter } from 'events'
import { Job, Queue, JobId, DoneCallback } from 'bull'

export default class BullWorker extends EventEmitter {
  public job!: Job | null
  constructor(
    protected delay: number,
    protected queue: Queue<any>,
    public work: (job: Job, done?: DoneCallback) => Promise<any>
  ) {
    super()
  }

  private async sleep(ms: number) {
    this.emit('info', `Sleeping for ${ms}ms`)
    return new Promise((resolve) => setTimeout(resolve, ms))
  }

  async poll(): Promise<void> {
    this.emit('info', 'Checking for new jobs')
    this.job = await this.queue.getNextJob()
    let result: null | [any, JobId]
    while (true) {
      await this.waitForJob()
      if (this.job === null) continue
      this.emit('info', `new job ${this.job.id} found`)
      this.emit('info', `starting work on ${this.job.id}`)
      try {
        await this.work(this.job)
        this.emit('info', `job ${this.job.id} completed`)
        result = await this.job.moveToCompleted('succeeded', true)
      } catch (e) {
        this.emit(
          'error',
          `error while working ${this.job.id} - attempts (${this.job.attemptsMade}) (${e.message})`
        )
        result = await this.job.moveToFailed(e, true)
        if (this.job.finishedOn) {
          this.emit('failed', this.job)
        }
      }
      try {
        await this.job.releaseLock()
      } catch (e) {
        this.emit(
          'error',
          `error releasing lock on ${this.job.id} - (${e.message}`
        )
      }
      if (Array.isArray(result)) {
        this.job = this.queue.nextJobFromJobData(result[0], result[1] as string)
      } else {
        this.job = null
      }
    }
  }

  async waitForJob(): Promise<void> {
    while (!this.job) {
      this.job = await this.queue.getNextJob()
      if (this.job) return
      await this.sleep(this.delay)
    }
  }
}
