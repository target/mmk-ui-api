import { subMinutes } from 'date-fns'
import { knex, Source } from '../models'
import SiteFactory from './factories/sites.factory'
import SourceFactory from './factories/sources.factory'
import { resetDB } from './utils'

import SiteService from '../services/site'

describe('Site Service', () => {
  let sourceSeed: Source
  beforeEach(async () => {
    await resetDB()
    sourceSeed = await SourceFactory.build().$query().insert()
  })
  afterAll(async () => {
    knex.destroy()
  })

  describe('getRunnable', () => {
    it('should return if run_every has passed', async () => {
      const last_run = subMinutes(new Date(), 65)
      const model = await SiteFactory.build({
        source_id: sourceSeed.id,
        last_run,
      })
        .$query()
        .insert()
      const actual = await SiteService.getRunnable()
      expect(actual.length).toBe(1)
      expect(actual[0].id).toBe(model.id)
    })
    it('should return nothing if run_every has not passed', async () => {
      const last_run = new Date()
      await SiteFactory.build({ source_id: sourceSeed.id, last_run })
        .$query()
        .insert()
      const actual = await SiteService.getRunnable()
      expect(actual.length).toBe(0)
    })
  })

  describe('create', () => {
    it('creates a site', async () => {
      const res = await SiteService.create({
        name: 'example site',
        active: true,
        run_every_minutes: 5,
        source_id: sourceSeed.id,
      })
      expect(res.name).toBe('example site')
    })
  })

  describe('view', () => {
    it('returns a site', async () => {
      const model = await SiteFactory.build({
        source_id: sourceSeed.id,
      })
        .$query()
        .insert()
      const res = await SiteService.view(model.id)
      expect(res.id).toBe(model.id)
    })
  })

  describe('update', () => {
    it('updates a site', async () => {
      const model = await SiteFactory.build({
        source_id: sourceSeed.id,
      })
        .$query()
        .insert()
      const res = await SiteService.update(model.id, { name: 'example update' })
      expect(res.name).toBe('example update')
    })
  })

  describe('destroy', () => {
    it('deletes a site', async () => {
      const model = await SiteFactory.build({
        source_id: sourceSeed.id,
      })
        .$query()
        .insert()
      const res = await SiteService.destroy(model.id)
      expect(res).toBe(1)
    })
  })
})
