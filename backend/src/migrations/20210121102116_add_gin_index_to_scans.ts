import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.raw(
    "CREATE INDEX scan_log_event_gin_idx ON scan_logs USING gin ( to_tsvector('english', event) )"
  )
}

export async function down(knex: Knex): Promise<void> {
  return knex.raw('DROP INDEX IF EXISTS scan_log_event_gin_idx')
}
