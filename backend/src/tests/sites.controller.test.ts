import request from 'supertest'
import { knex } from '../models'
import { Site, SiteAttributes, Source } from '../models'
import SiteFactory from './factories/sites.factory'
import SourceFactory from './factories/sources.factory'
import { makeSession, guestSession, resetDB } from './utils'

import Chance from 'chance'
const chance = new Chance()

const userSession = () =>
  makeSession({
    firstName: 'User',
    lastName: 'User',
    role: 'user',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
    exp: 0,
  }).app

const adminSession = () =>
  makeSession({
    firstName: 'Admin',
    lastName: 'User',
    exp: 0,
    role: 'admin',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
  }).app

describe('Sites Controller', () => {
  let seed: Site
  let sourceSeed: Source
  beforeEach(async () => {
    await resetDB()
    sourceSeed = await SourceFactory.build().$query().insert()
    seed = await SiteFactory.build({ source_id: sourceSeed.id })
      .$query()
      .insert()
  })
  afterAll(async () => {
    knex.destroy()
  })
  describe('GET /api/sites', () => {
    it('should get a list of Sites', async () => {
      const res = await request(adminSession()).get('/api/sites')
      expect(res.status).toBe(200)
      expect(res.body.results[0].id).toBe(seed.id)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/sites')
      expect(res.status).toBe(401)
    })
  })
  describe('GET /api/sites/:id', () => {
    it('should return the site for auth user', async () => {
      const res = await request(userSession()).get(`/api/sites/${seed.id}`)
      expect(res.status).toBe(200)
      expect(res.body.name).toBe(seed.name)
    })
    it('should return 401 for unauthenticated', async () => {
      const res = await request(guestSession().app).get(`/api/sites/${seed.id}`)
      expect(res.status).toBe(401)
    })
    it('should return not found for missing uuid', async () => {
      const res = await request(userSession()).get(
        `/api/sites/${chance.guid({ version: 4 })}`
      )
      expect(res.status).toBe(404)
    })
  })
  describe('POST /api/sites', () => {
    it('should create Site for admin user', async () => {
      const newSource: Source = await SourceFactory.build().$query().insert()
      const newSite: Site = SiteFactory.build({ source_id: newSource.id })
      const res = await request(adminSession())
        .post('/api/sites')
        .send({ site: newSite.toJSON() })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.name).toBe(newSite.name)
    })
    it('should reject Site for normal user', async () => {
      const newSource: Source = await SourceFactory.build().$query().insert()
      const newSite: Site = SiteFactory.build({ source_id: newSource.id })
      const res = await request(userSession())
        .post('/api/sites')
        .send({ site: newSite.toJSON() })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
    it('should reject Site for invalid name', async () => {
      const newSource: Source = await SourceFactory.build().$query().insert()
      const newSite: SiteAttributes = {
        source_id: newSource.id,
        name: '<script>bad</script>',
        run_every_minutes: 60,
        active: true,
      }
      const res = await request(adminSession())
        .post('/api/sites')
        .send({ site: newSite })
        .set('Accept', 'application/json')
      expect(res.status).toBe(400)
    })
  })
  describe('PUT /api/sites/:id', () => {
    it('should update Site for admin user', async () => {
      const update: SiteAttributes = {
        name: 'newName',
        active: true,
        run_every_minutes: 60,
        source_id: seed.source_id,
      }
      const res = await request(adminSession())
        .put(`/api/sites/${seed.id}`)
        .send({ site: update })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.name).toBe(update.name)
    })
    it('should reject on invalid name', async () => {
      const update: SiteAttributes = {
        name: '<script>bad</script>',
        active: true,
        run_every_minutes: 60,
        source_id: seed.source_id,
      }
      const res = await request(adminSession())
        .put(`/api/sites/${seed.id}`)
        .send({ site: update })
        .set('Accept', 'application/json')
      expect(res.status).toBe(400)
    })
    it('should reject update for normal user', async () => {
      const update: SiteAttributes = {
        name: 'newName',
        active: true,
        run_every_minutes: 60,
        source_id: seed.source_id,
      }
      const res = await request(userSession())
        .put(`/api/sites/${seed.id}`)
        .send({ site: update })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
  })
  describe('DELETE /api/sites/:id', () => {
    it('should delete Site for admin user', async () => {
      const res = await request(adminSession()).delete(`/api/sites/${seed.id}`)
      expect(res.status).toBe(200)
    })
    it('should reject delete Site for normal user', async () => {
      const res = await request(userSession()).delete(`/api/sites/${seed.id}`)
      expect(res.status).toBe(403)
    })
  })
})
