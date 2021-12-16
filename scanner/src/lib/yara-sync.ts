import yara from 'yara'
import { EventEmitter } from 'events'

type YaraRules = { rules: Array<Record<string, unknown>> }
type YaraVariableType = {
  variables?: Array<Record<string, unknown>>
  buffer: Buffer
}
type YaraScanResult = { rules: Array<Record<string, unknown>> }

interface YaraScanner {
  configure: (
    r: YaraRules,
    cb: (err: Error, warn: Array<unknown>) => void
  ) => void
  scan: (
    o: YaraVariableType,
    cb: (err: Error, result: YaraScanResult) => void
  ) => void
}

export default class YaraSync {
  events: EventEmitter
  scanner: YaraScanner
  constructor() {
    this.scanner = null
    this.events = new EventEmitter()
  }

  async initAsync(config: YaraRules): Promise<YaraScanner> {
    return new Promise((resolve, reject) => {
      yara.initialize((err: Error) => {
        if (err) {
          reject(err)
        }
        this.scanner = yara.createScanner()
        this.scanner.configure(config, (cErrors, cWarnings) => {
          if (cErrors) {
            reject(cErrors)
          }
          if (cWarnings && cWarnings.length) {
            this.events.emit('warn', cWarnings)
          }
          resolve(this.scanner)
        })
      })
    })
  }

  async scanAsync(options: YaraVariableType): Promise<YaraScanResult> {
    return new Promise((resolve, reject) => {
      this.scanner.scan(options, (error, result) => {
        if (error) {
          reject(error)
        }
        resolve(result)
      })
    })
  }
}
