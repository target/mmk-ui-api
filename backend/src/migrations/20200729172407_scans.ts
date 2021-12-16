import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('scans', (table) => {
    table.uuid('id').notNullable().primary().comment('Primary key (uuid)')
    table.string('state').notNullable().comment('Scan State')
    table.boolean('test').defaultTo(false)
    table.timestamp('created_at')
    table.timestamp('updated_at')
    table.uuid('source_id').references('id').inTable('sources').notNullable()
    table.uuid('site_id').references('id').inTable('sites').onDelete('CASCADE')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('scans')
}
