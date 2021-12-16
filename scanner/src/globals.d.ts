import { Job } from 'bull'
declare module 'hooks'
declare module 'yara'
declare module 'js-beautify'

declare module 'bull' {
  export interface Queue {
    nextJobFromJobData: (jobData: any, jobId?: string) => Job | null
  }
}
