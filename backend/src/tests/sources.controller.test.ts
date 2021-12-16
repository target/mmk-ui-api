import request from 'supertest'
import { knex, Source, SourceAttributes } from '../models'
import SourceFactory from './factories/sources.factory'
import SiteFactory from './factories/sites.factory'
import { makeSession, resetDB, guestSession } from './utils'

/*
import Chance from 'chance'
const chance = new Chance()
*/

const userSession = () =>
  makeSession({
    firstName: 'User',
    lastName: 'User',
    role: 'user',
    exp: 0,
    lanid: 'z000n00',
    email: 'foo@bar.com',
    isAuth: true,
  }).app

const adminSession = () =>
  makeSession({
    firstName: 'Admin',
    lastName: 'User',
    exp: 0,
    role: 'admin',
    lanid: 'z000n00',
    email: 'foo@bar.com',
    isAuth: true,
  }).app

describe('Sources Controller', () => {
  let seed: Source
  beforeEach(async () => {
    await resetDB()
    seed = await SourceFactory.build().$query().insert()
  })
  afterAll(async () => {
    knex.destroy()
  })
  describe('GET /api/sources', () => {
    it('should get a list of Sources', async () => {
      const res = await request(adminSession()).get('/api/sources')
      expect(res.status).toBe(200)
      expect(res.body.results[0].id).toBe(seed.id)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/sources')
      expect(res.status).toBe(401)
    })
    it('should return 403 for normal user', async () => {
      const res = await request(userSession()).get('/api/sources')
      expect(res.status).toBe(403)
    })
  })
  describe('GET /api/sources/:id', () => {
    it('should return the source for the admin user', async () => {
      const res = await request(adminSession()).get(`/api/sources/${seed.id}`)
      expect(res.status).toBe(200)
      expect(res.body.id).toBe(seed.id)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get(
        `/api/sources/${seed.id}`
      )
      expect(res.status).toBe(401)
    })
    it('should return 403 for normal user', async () => {
      const res = await request(userSession()).get(`/api/sources/${seed.id}`)
      expect(res.status).toBe(403)
    })
  })
  describe('POST /api/sources', () => {
    it('should create new Source for admin user', async () => {
      const newSource: Source = SourceFactory.build()
      const res = await request(adminSession())
        .post('/api/sources')
        .send({ source: newSource.toJSON() })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.name).toBe(newSource.name)
    })
    it('should reject create Source for normal user', async () => {
      const newSource: Source = SourceFactory.build()
      const res = await request(userSession())
        .post('/api/sources')
        .send({ source: newSource.toJSON() })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
    it('should reject create on invalid name', async () => {
      const newSource: SourceAttributes = {
        name: '<script>badname</script>',
        value: 'alert(123)',
      }
      const res = await request(adminSession())
        .post('/api/sources')
        .send({ source: newSource })
        .set('Accept', 'application/json')
      expect(res.status).toBe(400)
    })
  })
  describe('PUT /api/sources/:id', () => {
    it('should not allow update', async () => {
      const res = await request(adminSession())
        .put(`/api/sources/${seed.id}`)
        .send({ name: 'new name', value: 'new value' })
        .set('Accept', 'application/json')
      expect(res.status).toBe(404)
    })
  })
  describe('DELETE /api/sources/:id', () => {
    it('should delete Source for admin user', async () => {
      const res = await request(adminSession()).delete(
        `/api/sources/${seed.id}`
      )
      expect(res.status).toBe(200)
    })
    it('should reject if source is used by site', async () => {
      await SiteFactory.build({ source_id: seed.id }).$query().insert()
      const res = await request(adminSession()).delete(
        `/api/sources/${seed.id}`
      )
      expect(res.status).toBe(400)
    })
    it('should reject delet Source for normal user', async () => {
      const res = await request(userSession()).delete(`/api/sources/${seed.id}`)
      expect(res.status).toBe(403)
    })
  })
})
