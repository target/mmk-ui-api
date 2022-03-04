import AlertService from '../services/alert'
import AlertFactory from './factories/alert.factory'
import ScanFactory from './factories/scans.factory'
import SiteFactory from './factories/sites.factory'
import SourceFactory from './factories/sources.factory'
import { resetDB } from './utils'
import sub from 'date-fns/sub'

describe('Alert Service', () => {
  beforeEach(async () => {
    await resetDB()
  })
  describe('dateHist', () => {
    it('should return a grouping of alerts by hour', async () => {
      const seedSource = await SourceFactory.build()
        .$query()
        .insert()
        .returning('*')
      const seedSite = await SiteFactory.build({ source_id: seedSource.id })
        .$query()
        .insert()
        .returning('*')
      const seedScan = await ScanFactory.build({
        site_id: seedSite.id,
        source_id: seedSource.id
      })
        .$query()
        .insert()
        .returning('*')
      // current hour
      await AlertFactory.build({
        site_id: seedSite.id,
        scan_id: seedScan.id
      })
        .$query()
        .insert()
        .returning('*')
      // previous hour
      await AlertFactory.build({
        site_id: seedSite.id,
        scan_id: seedScan.id,
        created_at: sub(new Date(), { hours: 1 })
      })
        .$query()
        .insert()
        .returning('*')
      // previous hour
      await AlertFactory.build({
        site_id: seedSite.id,
        scan_id: seedScan.id,
        created_at: sub(new Date(), { hours: 1 })
      })
        .$query()
        .insert()
        .returning('*')
      // two hours ago
      await AlertFactory.build({
        site_id: seedSite.id,
        scan_id: seedScan.id,
        created_at: sub(new Date(), { hours: 2 })
      })
        .$query()
        .insert()
        .returning('*')
      const res = await AlertService.dateHist({
        starttime: sub(new Date(), { hours: 3 }),
        endtime: new Date(),
        interval: 1
      })
      expect(res.rows.length).toBe(4)
      expect(res.rows[0].count).toBe(1)
      expect(res.rows[1].count).toBe(2)
      expect(res.rows[2].count).toBe(1)
      expect(res.rows[3].count).toBe(0)
    })
  })
})
