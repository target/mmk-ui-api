import ScanService from '../services/scan'
import SiteFactory from './factories/sites.factory'
import ScanFactory from './factories/scans.factory'
import SourceFactory from './factories/sources.factory'
import ScanLogFactory from './factories/scan_log.factory'
import { resetDB } from './utils'
import { ScanAttributes } from '../models/scans'
import { WebRequestEvent } from '@merrymaker/types'

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

describe('Scan Service', () => {
  beforeEach(async () => {
    await resetDB()
  })
  it('deletes scans older than 5 days', async () => {
    const now = new Date()
    const sixDaysAgo = new Date(new Date().setDate(now.getDate() - 6))
    const sourceModel = await SourceFactory.build()
      .$query()
      .insert()
    const siteModel = await SiteFactory.build({ source_id: sourceModel.id })
      .$query()
      .insert()
    await ScanFactory.build({
      created_at: sixDaysAgo,
      source_id: sourceModel.id,
      site_id: siteModel.id
    })
      .$query()
      .insert()
    await ScanFactory.build({
      source_id: sourceModel.id,
      site_id: siteModel.id
    })
      .$query()
      .insert()
    const total = await ScanService.purge(5)
    expect(total).toBe(1)
  })
  it('deletes test scans 6 hours or older', async () => {
    const now = new Date()
    const sixHoursAgo = new Date(new Date().setHours(now.getHours() - 6.1))
    const sourceModel = await SourceFactory.build()
      .$query()
      .insert()
    const siteModel = await SiteFactory.build({ source_id: sourceModel.id })
      .$query()
      .insert()
    await ScanFactory.build({
      created_at: sixHoursAgo,
      source_id: sourceModel.id,
      site_id: siteModel.id,
      test: true
    })
      .$query()
      .insert()
    await ScanFactory.build({
      source_id: sourceModel.id,
      site_id: siteModel.id
    })
      .$query()
      .insert()
    const total = await ScanService.purgeTests(6)
    expect(total).toBe(1)
  })
  describe('isActive', () => {
    it('returns true if scan is active', async () => {
      const activeScan = await helper({ state: 'active' })
      const res = await ScanService.isActive(activeScan.id)
      expect(res).toBe(true)
    })
    it('returns false if scan is not active', async () => {
      const activeScan = await helper({ state: 'done' })
      const res = await ScanService.isActive(activeScan.id)
      expect(res).toBe(false)
    })
  })
  describe('isBulkActive', () => {
    it('returns true if bulk scan is active', async () => {
      const activeScanA = await helper({ state: 'active' })
      const activeScanB = await helper({ state: 'done' })
      const res = await ScanService.isBulkActive([
        activeScanA.id,
        activeScanB.id
      ])
      expect(res).toBe(true)
    })
    it('returns false if bulk scan is active', async () => {
      const activeScanA = await helper({ state: 'done' })
      const activeScanB = await helper({ state: 'done' })
      const res = await ScanService.isBulkActive([
        activeScanA.id,
        activeScanB.id
      ])
      expect(res).toBe(false)
    })
  })
  describe('view', () => {
    it('returns a scan', async () => {
      const viewScan = await helper()
      const res = await ScanService.view(viewScan.id)
      expect(res.id).toBe(viewScan.id)
    })
    it('throws an error if missing', async () => {
      let err: Error
      try {
        await ScanService.view('60136ed0-355e-4a67-afeb-3ffdc9589891')
      } catch (e) {
        err = e
      }
      expect(err).not.toBeUndefined
    })
  })
  describe('destory', () => {
    it('destorys a scan', async () => {
      const viewScan = await helper()
      const res = await ScanService.destroy(viewScan.id)
      expect(res).toBe(1)
    })
    it('throws an error if missing', async () => {
      let err: Error
      try {
        await ScanService.destroy('60136ed0-355e-4a67-afeb-3ffdc9589891')
      } catch (e) {
        err = e
      }
      expect(err).not.toBeUndefined
    })
  })
  describe('findAndExpire', () => {
    it('finds and expires a scan', async () => {
      const today = new Date()
      today.setHours(today.getHours() - 1.1)
      await helper({ state: 'running', created_at: new Date() })
      const longRunning = await helper({ state: 'running', created_at: today })
      const res = await ScanService.findAndExpire(60)
      expect(res).toBe(1)
      const expiredRunning = await longRunning.$query()
      expect(expiredRunning.state).toBe('expired')
    })
  })
  describe('groupDomainRequests', () => {
    it('returns an array of grouped domains', async () => {
      const viewScan = await helper()
      await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'http://www.google.com/foo.js' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'http://www.yahoo.com/moo.html' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      const actual = await ScanService.groupLogs(viewScan.id, {
        entry: 'request',
        composites: [ScanService.domainComposite]
      })
      expect(actual).toEqual({
        domain: {
          'www.google.com': 1,
          'www.yahoo.com': 1
        }
      })
    })
    it('handles invalid URLs', async () => {
      const viewScan = await helper()
      await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'data:image/gif;base64,R0lC' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'http://www.yahoo.com/moo.html' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      const actual = await ScanService.groupLogs(viewScan.id, {
        entry: 'request',
        composites: [ScanService.domainComposite]
      })
      expect(actual).toEqual({
        domain: {
          'www.yahoo.com': 1
        }
      })
    })
    it('should increment duplicate', async () => {
      const viewScan = await helper()
      await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'http://www.yahoo.com/moo.html' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'http://www.yahoo.com/bar.html' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      const actual = await ScanService.groupLogs(viewScan.id, {
        entry: 'request',
        composites: [ScanService.domainComposite]
      })
      expect(actual).toEqual({
        domain: {
          'www.yahoo.com': 2
        }
      })
    })
  })
  describe('urlComposite', () => {
    it('should return href', async () => {
      const viewScan = await helper()
      const sLog = await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'http://www.yahoo.com/moo.html' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      const actual = ScanService.urlComposite.group(sLog)
      expect(actual).toEqual((sLog.event as WebRequestEvent).url)
    })
    it('should return undefined on invalid', async () => {
      const viewScan = await helper()
      const sLog = await ScanLogFactory.build({
        entry: 'request',
        event: { url: 'data:image/gif;base64,R0lC' } as WebRequestEvent,
        scan_id: viewScan.id,
        created_at: new Date('1955-11-12T06:38:00.000Z')
      })
        .$query()
        .insert()
      const actual = ScanService.urlComposite.group(sLog)
      expect(actual).toEqual(undefined)
    })
  })
})
