import { resetDB } from './utils'
import SeenString from '../models/seen_strings'
import SeenStringFactory from './factories/seen_strings.factory'
import SeenStringService from '../services/seen_string'

describe('Seen String Service', () => {
  beforeEach(async () => {
    await resetDB()
  })
  describe('create', () => {
    it('creates a seen string', async () => {
      const res = await SeenStringService.create({
        type: 'fqdn',
        key: 'example.com',
      })
      expect(res.key).toBe('example.com')
    })
  })
  describe('view', () => {
    it('returns a seen string', async () => {
      const seenString = await SeenStringFactory.build({ key: 'cow' })
        .$query()
        .insert()
      const res = await SeenStringService.view(seenString.id)
      expect(res.key).toBe('cow')
    })
  })
  describe('update', () => {
    it('updates a seen string', async () => {
      const seenString = await SeenStringFactory.build({ key: 'cow' })
        .$query()
        .insert()
      const res = await SeenStringService.update(seenString.id, { key: 'moo' })
      expect(res.key).toBe('moo')
    })
  })
  describe('destroy', () => {
    it('deletes a seen string', async () => {
      const seenString = await SeenStringFactory.build({ key: 'cow' })
        .$query()
        .insert()
      const res = await SeenStringService.destroy(seenString.id)
      expect(res).toBe(1)
    })
  })
  describe('purgeDBCache', () => {
    let seenStringSet: SeenString[]
    beforeAll(async () => {
      const daysAgo = new Date()
      daysAgo.setDate(daysAgo.getDate() - 3)
      await SeenStringFactory.build({ key: 'oldone', last_cached: daysAgo })
        .$query().insert()
      await SeenStringFactory.build({ key: 'newone' })
        .$query().insert()
      await SeenStringFactory.build({ key: 'nullone', last_cached: null })
        .$query().insert()
      await SeenStringService.purgeDBCache(2)
      seenStringSet = await SeenString.query()
    })
    it('deletes seen strings older than 2 days', () => {
      expect(seenStringSet.every(s => s.key !== 'oldone')).toEqual(true)
    })
    it('deletes seen strings with null last_cached', () => {
      expect(seenStringSet.every(s => s.key !== 'nullone')).toEqual(true)
    })
    it('does not delete seen strings under 2 days', () => {
      expect(seenStringSet.some(s => s.key === 'newone')).toEqual(true)
    })
  })
})
