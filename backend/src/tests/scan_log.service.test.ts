import Chance from 'chance'
import { Job } from 'bull'
import ScanLogService from '../services/scan_logs'
import ScanService from '../services/scan'
import SiteFactory from './factories/sites.factory'
import ScanFactory from './factories/scans.factory'
import SourceFactory from './factories/sources.factory'
import ScanLogFactory from './factories/scan_log.factory'
import { resetDB } from './utils'
import Scan, { ScanAttributes } from '../models/scans'
import { RuleAlert, RuleAlertEvent, WebRequestEvent } from '@merrymaker/types'

const chance = Chance.Chance()

const helper = async (scanAttrs: Partial<ScanAttributes> = {}) => {
  const sourceModel = await SourceFactory.build()
    .$query()
    .insert()
  const siteModel = await SiteFactory.build({ source_id: sourceModel.id })
    .$query()
    .insert()
  return ScanFactory.build({
    source_id: sourceModel.id,
    site_id: siteModel.id,
    ...scanAttrs
  })
    .$query()
    .insert()
}

describe('Scan Log Service', () => {
  beforeEach(async () => {
    await resetDB()
  })
  describe('countByScanID', () => {
    it('should return total', async () => {
      const viewScan = await helper()
      for (let i = 0; i < 10; i += 1) {
        await ScanLogFactory.build({
          entry: 'request',
          event: { url: chance.url() } as WebRequestEvent,
          scan_id: viewScan.id,
          created_at: new Date()
        })
          .$query()
          .insert()
      }
      const actual = await ScanLogService.countByScanID(viewScan.id, 'request')
      expect(actual).toBe(10)
    })
    it('should scope on event value', async () => {
      const viewScan = await helper()
      await ScanLogFactory.build({
        entry: 'rule-alert',
        event: { alert: false } as RuleAlert,
        scan_id: viewScan.id,
        created_at: new Date()
      })
        .$query()
        .insert()
      await ScanLogFactory.build({
        entry: 'rule-alert',
        event: { alert: true } as RuleAlert,
        scan_id: viewScan.id,
        created_at: new Date()
      })
        .$query()
        .insert()
      const actual = await ScanLogService.countByScanID(
        viewScan.id,
        'rule-alert',
        ScanService.ruleAlertEvent
      )
      expect(actual).toBe(1)
    })
  })
  describe('work', () => {
    it('should create a log entry', async () => {
      const viewScan = await helper()
      const eventResult = ScanLogFactory.build({
        entry: 'rule-alert',
        rule: 'test.rule',
        event: {
          name: 'test-rule',
          level: 'test',
          message: 'some test message',
          context: { foo: 'bar' },
          alert: true
        } as RuleAlert,
        scan_id: viewScan.id,
        created_at: new Date()
      } as RuleAlertEvent)
      const logEntry = await ScanLogService.work({ data: eventResult } as Job)
      expect(logEntry.scan_id).toBe(viewScan.id)
    })
    it('should update scan state to "active"', async () => {
      const viewScan = await helper({ state: 'scheduled' })
      const eventResult = ScanLogFactory.build({
        entry: 'active',
        event: { message: 'running' },
        scan_id: viewScan.id,
        created_at: new Date()
      })
      await ScanLogService.work({ data: eventResult } as Job)
      const updatedScan = await viewScan.$query()
      expect(updatedScan.state).toBe('active')
    })
    it('should update scan state to "completed"', async () => {
      const viewScan = await helper({ state: 'active' })
      const eventResult = ScanLogFactory.build({
        entry: 'complete',
        event: { message: 'completed scan' },
        scan_id: viewScan.id,
        created_at: new Date()
      })
      await ScanLogService.work({ data: eventResult } as Job)
      const updatedScan = await viewScan.$query()
      expect(updatedScan.state).toBe('completed')
    })
    it('should update scan state to "failed"', async () => {
      const viewScan = await helper({ state: 'active' })
      const eventResult = ScanLogFactory.build({
        entry: 'failed',
        event: { message: 'completed scan' },
        scan_id: viewScan.id,
        created_at: new Date()
      })
      await ScanLogService.work({
        failedReason: 'test failure',
        data: eventResult,
        queue: { name: 'test' }
      } as Job)
      const updatedScan = await viewScan.$query()
      expect(updatedScan.state).toBe('failed')
    })
    it('should throw exception on missing value', async () => {
      let err: Error
      const viewScan = await helper()
      const eventResult = ScanLogFactory.build({
        entry: 'rule-alert',
        rule: 'test.rule',
        event: {
          name: 'test-rule',
          level: 'test',
          context: { foo: 'bar' },
          alert: true
        } as RuleAlert,
        scan_id: viewScan.id,
        created_at: new Date()
      } as RuleAlertEvent)
      try {
        await ScanLogService.work({ data: eventResult } as Job)
      } catch (e) {
        err = e
      }
      expect(err).not.toBe(null)
    })
  })
  describe('handleAlert', () => {
    let testScan: Scan
    beforeEach(async () => {
      testScan = await helper()
    })
    it('should return on alert is false', async () => {
      const eventResult: RuleAlertEvent = {
        entry: 'rule-alert',
        rule: 'test.rule',
        level: 'info',
        event: {
          name: 'test-rule',
          level: 'test',
          context: { foo: 'bar' },
          alert: false
        },
        scan_id: testScan.id,
        created_at: new Date()
      }
      const result = await ScanLogService.handleAlert(eventResult)
      expect(result.result).toBe('event.alert is false')
    })

    it('should throw exception if site_id is invalid', async () => {
      const eventResult: RuleAlertEvent = {
        entry: 'rule-alert',
        rule: 'test.rule',
        level: 'info',
        event: {
          name: 'test-rule',
          level: 'test',
          context: { foo: 'bar' },
          alert: true
        },
        scan_id: chance.guid(),
        created_at: new Date()
      }
      let err: Error
      try {
        await ScanLogService.handleAlert(eventResult)
      } catch (e) {
        err = e
      }
      expect(err.message).toBe('invalid scan_id')
    })
    it('should schedule a Job and insert an Alert', async () => {
      const eventResult: RuleAlertEvent = {
        entry: 'rule-alert',
        rule: 'test.rule',
        level: 'info',
        event: {
          name: 'test-rule',
          level: 'test',
          message: 'testing this rule',
          context: { foo: 'bar' },
          alert: true
        },
        scan_id: testScan.id,
        created_at: new Date()
      }
      const result = await ScanLogService.handleAlert(eventResult)
      expect(result.result).toBe('alerted')
      expect(result.alertEvent).not.toBeUndefined()
      expect(result.job).not.toBeUndefined()
    })
  })
})
