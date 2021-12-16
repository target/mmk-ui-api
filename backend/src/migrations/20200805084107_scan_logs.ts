import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('scan_logs', (table) => {
    table.uuid('id').notNullable().primary().comment('Primary key (uuid)')
    table.text('entry').notNullable().comment('Log entry')
    table.string('level').notNullable().comment('Log level')
    table.json('event').comment('Scan event payload')
    table.timestamp('created_at')
    table
      .uuid('scan_id')
      .references('id')
      .inTable('scans')
      .notNullable()
      .onDelete('CASCADE')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('scan_logs')
}
