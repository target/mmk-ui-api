import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.raw('CREATE INDEX scan_log_scan_idx ON scan_logs(scan_id)')
}

export async function down(knex: Knex): Promise<void> {
  return knex.raw('DROP INDEX scan_log_scan_idx')
}
