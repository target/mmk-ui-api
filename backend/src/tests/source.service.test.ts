import SourceService from '../services/source'
import SecretFactory from './factories/secrets.factory'
import { resetDB } from './utils'
import { SourceSecret, Secret, Source } from '../models'

describe('Source Service', () => {
  beforeEach(async () => {
    await resetDB()
  })
  describe('create', () => {
    it('creates a new source without secrets', async () => {
      const source = await SourceService.create({
        name: 'foobar',
        value: 'moocar',
        secret_ids: [],
      })
      expect(source.name).toBe('foobar')
    })
    it('creates a new source with secrets', async () => {
      const secret = await SecretFactory.build().$query().insert()
      const source = await SourceService.create({
        name: 'foobar',
        value: 'moocar',
        secret_ids: [secret.id],
      })
      expect(source.name).toBe('foobar')
      const related = SourceSecret.query().where({ source_id: source.id })
      expect(related).not.toBe(null)
    })
    it('throws error on invalid secret id', async () => {
      let err: Error
      try {
        await SourceService.create({
          name: 'foobar1',
          value: 'moocar',
          secret_ids: ['12345'],
        })
      } catch (e) {
        err = e
      }
      expect(err).not.toBe(null)
      // rollback transaction works
      const record = await Source.query().where({ name: 'foobar1' })
      expect(record.length).toBe(0)
    })
    it('populate cache with resolved secrets', async () => {
      const secret = await SecretFactory.build().$query().insert()
      const source = await SourceService.create({
        name: 'foobar',
        value: `call("__${secret.name}__")`,
        secret_ids: [secret.id],
      })
      const cached = await SourceService.getCache(source.id)
      expect(cached).toBe(`call("${secret.value}")`)
    })
  })
  describe('resolve', () => {
    it('populates secrets', async () => {
      const secrets: Secret[] = []
      secrets.push(
        SecretFactory.build({
          name: 'secretName',
          value: 'foo123',
        })
      )
      const actual = SourceService.resolve('call("__secretName__")', secrets)
      expect(actual).toBe('call("foo123")')
    })
    it('populates multiple secrets', async () => {
      const secrets: Secret[] = []
      secrets.push(SecretFactory.build())
      secrets.push(SecretFactory.build())
      const actual = SourceService.resolve(
        `
          call("__${secrets[0].name}__")
          call("__${secrets[1].name}__")
      `,
        secrets
      )
      expect(actual).toBe(`
          call("${secrets[0].value}")
          call("${secrets[1].value}")
      `)
    })
  })
})
