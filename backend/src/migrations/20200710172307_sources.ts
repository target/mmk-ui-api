import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('sources', (table) => {
    table.uuid('id').notNullable().primary().comment('Primary key (uuid)')
    table.string('name').notNullable().unique().comment('Name of source')
    table.boolean('test').defaultTo(false)
    table.text('value').notNullable().comment('Source Code to Run')
    table.timestamp('created_at')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('sources')
}
