import { Knex } from 'knex'
import { Source, SourceSecret, Secret, SourceAttributes } from '../models'
import { redisClient } from '../repos/redis'

/** Request to create or update a source */
type SourceRequest = SourceAttributes & { secret_ids: string[] }

/**
 * associateSecrets
 *
 * Maps a set of `secrets` to a source based on IDs.
 * Runs within knex Transaction to ensure data integrity.
 */
const associateSecrets = async (
  source_id: string,
  secret_ids: string[],
  trx: Knex.Transaction
) =>
  SourceSecret.query(trx).insert(
    secret_ids.map((secret_id) => ({
      source_id,
      secret_id,
    }))
  )

/**
 * create
 *
 *  Creates a new `source` instance with optional `secrets`
 *
 *  Source and SourceSecret inserts are wrapped inside a DB transaction
 *  to ensure both complete when present
 */
export async function create(newSource: SourceRequest): Promise<Source> {
  const record = await Source.transaction(async (trx) => {
    const sourceRecord = await Source.query(trx).insert(
      Source.insertAble().reduce(
        (obj, key) => ({ ...obj, [key]: newSource[key] }),
        {}
      )
    )
    // Check if source has secrets
    if (
      newSource.secret_ids &&
      Array.isArray(newSource.secret_ids) &&
      newSource.secret_ids.length > 0
    ) {
      await associateSecrets(sourceRecord.id, newSource.secret_ids, trx)
    }

    return sourceRecord
  })

  await cache(record.id)
  return record
}

/**
 * getCache
 *
 * Fetches the cached `value` for a source by ID
 */
export async function getCache(id: string): Promise<string | null> {
  return redisClient.get(`source:${id}`)
}

/**
 * cache
 *
 * Stores the `value` for a source by ID.
 * Interpolates current associated secrets.
 */
export async function cache(id: string): Promise<void> {
  const fullSource = await Source.query()
    .withGraphFetched('secrets')
    .findById(id)
  const value = resolve(fullSource.value, fullSource.secrets)
  await redisClient.set(`source:${id}`, value)
  return
}

/**
 * clearCache
 *
 * Deletes source value from cache
 */
export async function clearCache(id: string): Promise<number> {
  return redisClient.del(`source:${id}`)
}

/**
 * resolve
 *
 * Replaces `__SECRET__` with value given a set a `secrets`
 *
 */
export function resolve(source: string, secrets: Secret[]): string {
  secrets.forEach((secret) => {
    source = source.replace(`__${secret.name}__`, secret.value)
  })
  return source
}

const view = async (id: string): Promise<Source> =>
  Source.query().findById(id).throwIfNotFound()

const destroy = async (id: string): Promise<number> =>
  Source.query().deleteById(id)

export default {
  create,
  view,
  destroy,
  resolve,
  cache,
  getCache,
  clearCache,
}
