import request from 'supertest'
import { knex, Source } from '../models'
import { Scan, Site } from '../models'

import SiteFactory from './factories/sites.factory'
import ScanFactory from './factories/scans.factory'
import SourceFactory from './factories/sources.factory'

import { makeSession, resetDB } from './utils'

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

describe('Scan Controller', () => {
  let seedA: Scan
  let siteSeedA: Site
  let siteSeedB: Site
  let sourceSeed: Source
  beforeEach(async () => {
    await resetDB()
    // Site needs a source
    sourceSeed = await SourceFactory.build().$query().insert()
    // Scan needs a valid Site
    siteSeedA = await SiteFactory.build({ source_id: sourceSeed.id })
      .$query()
      .insert()
    siteSeedB = await SiteFactory.build({ source_id: sourceSeed.id })
      .$query()
      .insert()
    seedA = await ScanFactory.build({
      site_id: siteSeedA.id,
      source_id: sourceSeed.id,
    })
      .$query()
      .insert()
    await ScanFactory.build({ site_id: siteSeedB.id, source_id: sourceSeed.id })
      .$query()
      .insert()
  })
  afterAll(async () => {
    knex.destroy()
  })
  describe('GET /api/scans', () => {
    it('should get a list of Scans', async () => {
      const res = await request(userSession()).get('/api/scans')
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(2)
    })
    it('should get a list of Scans for :site_id', async () => {
      const res = await request(userSession())
        .get('/api/scans')
        .query({ site_id: siteSeedA.id })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].id).toBe(seedA.id)
    })
  })
  describe('GET /api/scans/:id', () => {
    it('should get a Scan record', async () => {
      const res = await request(userSession()).get(`/api/scans/${seedA.id}`)
      expect(res.status).toBe(200)
      expect(res.body.id).toBe(seedA.id)
    })
  })
  describe('DELETE /api/scans/:id', () => {
    it('should error if scan is active', async () => {
      const activeScan = await ScanFactory.build({
        site_id: siteSeedA.id,
        source_id: sourceSeed.id,
        state: 'active',
      })
        .$query()
        .insert()
      const res = await request(adminSession()).delete(
        `/api/scans/${activeScan.id}`
      )
      expect(res.status).toBe(422)
    })
  })
  describe('POST /api/scans/bulk_delete', () => {
    it('should bulk delete scans', async () => {
      const res = await request(adminSession())
        .post('/api/scans/bulk_delete')
        .send({ scans: { ids: [seedA.id] } })
      expect(res.status).toBe(200)
    })
  })
})
