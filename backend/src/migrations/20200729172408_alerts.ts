import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('alerts', (table) => {
    table.uuid('id').notNullable().primary().comment('Primary key (uuid)')
    table.string('rule').notNullable()
    table.uuid('scan_id').references('id').inTable('scans').onDelete('SET NULL')
    table.uuid('site_id').references('id').inTable('sites').onDelete('SET NULL')
    table.text('message').notNullable().comment('Alert message')
    table.timestamp('created_at')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('alerts')
}
