import { PathItem, ajv } from 'aejo'
import Chance from 'chance'
import { guestSession, makeSession, resetDB } from './utils'
import AlertFactory from './factories/alert.factory'
import ScanFactory from './factories/scans.factory'
import SiteFactory from './factories/sites.factory'
import SourceFactory from './factories/sources.factory'
import { Alert, knex } from '../models'
import request from 'supertest'
import { uuidFormat } from '../api/crud/schemas'

const chance = new Chance()

const userSession = () =>
  makeSession({
    firstName: 'User',
    lastName: 'User',
    role: 'user',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
    exp: 1,
  })

const adminSession = () =>
  makeSession({
    firstName: 'Admin',
    lastName: 'User',
    role: 'admin',
    exp: 0,
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
  })

describe('Alert Controller', () => {
  let seed: Alert
  let api: PathItem
  beforeAll(() => {
    api = adminSession().paths
  })
  beforeEach(async () => {
    await resetDB()
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
      source_id: seedSource.id,
    })
      .$query()
      .insert()
      .returning('*')
    seed = await AlertFactory.build({
      site_id: seedSite.id,
      scan_id: seedScan.id,
    })
      .$query()
      .insert()
      .returning('*')
  })
  afterAll(async () => knex.destroy)
  describe('GET /api/alerts', () => {
    it('should get a list of Alerts', async () => {
      const res = await request(userSession().app).get('/api/alerts')
      expect(res.status).toBe(200)
      expect(res.body.results[0].id).toBe(seed.id)
      const validate = ajv.compile(
        api['/api/alerts/'].get.responses['200'].content['application/json']
          .schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/alerts')
      expect(res.status).toBe(401)
    })
    it('should filter by "scan_id"', async () => {
      const res = await request(adminSession().app)
        .get('/api/alerts')
        .query({ scan_id: chance.guid({ version: 4 }) })
      expect(res.body.total).toBe(0)
    })
    it('should eager load site name', async () => {
      const res = await request(adminSession().app)
        .get('/api/alerts')
        .query({ 'eager[]': 'site' })
      expect(res.body.results[0].site.name).not.toBe(undefined)
    })
    it('should return validation error on invalid eager value', async () => {
      const res = await request(adminSession().app)
        .get('/api/alerts')
        .query({ 'eager[]': 'scan' })
      expect(res.status).toBe(422)
      expect(res.body.type).toBe('ValidationError')
      expect(res.body.data.path).toBe('/eager/0')
    })
  })
  describe('GET /api/alerts/:id', () => {
    it('should get Alert by ID', async () => {
      const res = await request(userSession().app).get(`/api/alerts/${seed.id}`)
      expect(res.status).toBe(200)
      expect(res.body.id).toBe(seed.id)
      const validate = ajv.compile(
        api[`/api/alerts/:id(${uuidFormat})`].get.responses['200'].content[
          'application/json'
        ].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return 404 for invalid ID', async () => {
      const res = await request(userSession().app).get(
        `/api/alerts/${chance.guid({ version: 4 })}`
      )
      expect(res.status).toBe(404)
    })
  })
  describe('GET /api/alerts/distinct', () => {
    it('should get distinct alert column values', async () => {
      const res = await request(adminSession().app)
        .get('/api/alerts/distinct')
        .query({ column: 'rule' })
      expect(res.status).toBe(200)
      expect(res.body[0]).toEqual({ rule: seed.rule })
      const validate = ajv.compile(
        api['/api/alerts/distinct'].get.responses['200'].content[
          'application/json'
        ].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return validation error on invalid column name', async () => {
      const res = await request(adminSession().app)
        .get('/api/alerts/distinct')
        .query({ column: 'bad' })
      expect(res.status).toBe(422)
    })
  })
  describe('DELETE /api/alerts/:id', () => {
    it('should delete Alert by ID', async () => {
      const res = await request(adminSession().app).delete(
        `/api/alerts/${seed.id}`
      )
      expect(res.status).toBe(200)
      const validate = ajv.compile(
        api[`/api/alerts/:id(${uuidFormat})`].delete.responses['200'].content[
          'application/json'
        ].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should not allow user to delete', async () => {
      const res = await request(userSession().app).delete(
        `/api/alerts/${seed.id}`
      )
      expect(res.status).toBe(403)
    })
  })
})
