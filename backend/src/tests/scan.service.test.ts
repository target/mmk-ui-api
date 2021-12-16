import ScanService from '../services/scan'
import SiteFactory from './factories/sites.factory'
import ScanFactory from './factories/scans.factory'
import SourceFactory from './factories/sources.factory'
import { resetDB } from './utils'
import { ScanAttributes } from '../models/scans'

const helper = async (scanAttrs: Partial<ScanAttributes> = {}) => {
  const sourceModel = await SourceFactory.build().$query().insert()
  const siteModel = await SiteFactory.build({ source_id: sourceModel.id })
    .$query()
    .insert()
  return ScanFactory.build({
    source_id: sourceModel.id,
    site_id: siteModel.id,
    ...scanAttrs,
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
    const fiveDaysAgo = new Date(new Date().setDate(now.getDate() - 5))
    const sourceModel = await SourceFactory.build().$query().insert()
    const siteModel = await SiteFactory.build({ source_id: sourceModel.id })
      .$query()
      .insert()
    await ScanFactory.build({
      created_at: fiveDaysAgo,
      source_id: sourceModel.id,
      site_id: siteModel.id,
    })
      .$query()
      .insert()
    await ScanFactory.build({
      source_id: sourceModel.id,
      site_id: siteModel.id,
    })
      .$query()
      .insert()
    const total = await ScanService.purge(5)
    expect(total).toBe(1)
  })
  it('deletes test scans 6 hours or older', async () => {
    const now = new Date()
    const sixHoursAgo = new Date(new Date().setHours(now.getHours() - 6))
    const sourceModel = await SourceFactory.build().$query().insert()
    const siteModel = await SiteFactory.build({ source_id: sourceModel.id })
      .$query()
      .insert()
    await ScanFactory.build({
      created_at: sixHoursAgo,
      source_id: sourceModel.id,
      site_id: siteModel.id,
      test: true,
    })
      .$query()
      .insert()
    await ScanFactory.build({
      source_id: sourceModel.id,
      site_id: siteModel.id,
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
        activeScanB.id,
      ])
      expect(res).toBe(true)
    })
    it('returns false if bulk scan is active', async () => {
      const activeScanA = await helper({ state: 'done' })
      const activeScanB = await helper({ state: 'done' })
      const res = await ScanService.isBulkActive([
        activeScanA.id,
        activeScanB.id,
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
})
