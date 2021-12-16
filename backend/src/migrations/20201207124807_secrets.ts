import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('secrets', (table) => {
    table.uuid('id').unique().primary().comment('Secret ID')
    table.string('name', 255).unique().notNullable()
    table.text('value').notNullable()
    table.string('type').notNullable()
    table.timestamp('created_at')
    table.timestamp('updated_at')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('secrets')
}
