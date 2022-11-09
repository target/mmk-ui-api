import request from 'supertest'
import Chance from 'chance'
import { knex, Scan, Site, Source, ScanLog } from '../models'
import { makeSession, resetDB, guestSession } from './utils'
import SiteFactory from './factories/sites.factory'
import ScanFactory from './factories/scans.factory'
import SourceFactory from './factories/sources.factory'
import ScanLogFactory from './factories/scan_log.factory'
import { PathItem, ajv } from 'aejo'

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

describe('ScanLog Controller', () => {
  let seedA: ScanLog
  let scanSeedA: Scan
  let siteSeedA: Site
  let siteSeedB: Site
  let sourceSeed: Source
  let api: PathItem
  beforeAll(() => {
    api = userSession().paths
  })
  beforeEach(async () => {
    await resetDB()
    sourceSeed = await SourceFactory.build().$query().insert()
    siteSeedA = await SiteFactory.build({ source_id: sourceSeed.id })
      .$query()
      .insert()
    siteSeedB = await SiteFactory.build({ source_id: sourceSeed.id })
      .$query()
      .insert()
    scanSeedA = await ScanFactory.build({
      site_id: siteSeedA.id,
      source_id: sourceSeed.id,
    })
      .$query()
      .insert()
    const scanSeedB = await ScanFactory.build({
      site_id: siteSeedB.id,
      source_id: sourceSeed.id,
    })
      .$query()
      .insert()
    seedA = await ScanLogFactory.build({
      entry: 'page-error',
      event: { message: 'found artifactA' },
      scan_id: scanSeedA.id,
      created_at: new Date('1955-11-12T06:38:00.000Z'),
    })
      .$query()
      .insert()
    await ScanLogFactory.build({
      scan_id: scanSeedB.id,
    })
      .$query()
      .insert()
  })
  afterAll(async () => {
    knex.destroy()
  })
  describe('GET /api/scan_logs', () => {
    it('should get a list of ScanLogs', async () => {
      const res = await request(userSession().app).get('/api/scan_logs')
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(2)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/scan_logs')
      expect(res.status).toBe(401)
    })
    it('should return only matching scan_id', async () => {
      const res = await request(userSession().app)
        .get('/api/scan_logs')
        .query({ scan_id: scanSeedA.id })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].scan_id).toBe(scanSeedA.id)
    })
    it('should return only matching search results', async () => {
      const res = await request(userSession().app)
        .get('/api/scan_logs')
        .query({ search: 'artifactA' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].id).toBe(seedA.id)
    })
    it('should not return any results', async () => {
      const res = await request(userSession().app)
        .get('/api/scan_logs')
        .query({ search: 'artifactB' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(0)
    })
    it('should return only on matching entry type', async () => {
      const res = await request(userSession().app)
        .get('/api/scan_logs')
        .query({ 'entry[]': 'page-error' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].id).toBe(seedA.id)
    })
    it('should return only on matching from date', async () => {
      const res = await request(userSession().app)
        .get('/api/scan_logs')
        .query({ from: '1955-11-13T06:38:00.000Z' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].id).not.toBe(seedA.id)
    })
  })
  describe('GET /api/scan_logs/:id', () => {
    it('should return a ScanLog by id', async () => {
      const res = await request(userSession().app).get(
        `/api/scan_logs/${seedA.id}`
      )
      expect(res.body.id).toBe(seedA.id)
      const validate = ajv.compile(
        api['/api/scan_logs/{id}'].get.responses['200'].content[
          'application/json'
        ].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return 404 for invalid id', async () => {
      const res = await request(userSession().app).get(
        `/api/scan_logs/${chance.guid({ version: 4 })}`
      )
      expect(res.status).toBe(404)
    })
  })
  describe('GET /api/scan_logs/:id/distinct', () => {
    it('should get distinct ScanLog column values', async () => {
      const res = await request(userSession().app)
        .get(`/api/scan_logs/${scanSeedA.id}/distinct`)
        .query({ column: 'entry' })
      expect(res.status).toBe(200)
      expect(res.body[0]).toEqual({ entry: seedA.entry })
      const validate = ajv.compile(
        api['/api/scan_logs/{id}/distinct'].get.responses['200']
          .content['application/json'].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return validation error on invalid column name', async () => {
      const res = await request(userSession().app)
        .get(`/api/scan_logs/${scanSeedA.id}/distinct`)
        .query({ column: 'bad' })
      expect(res.status).toBe(422)
    })
  })
})
