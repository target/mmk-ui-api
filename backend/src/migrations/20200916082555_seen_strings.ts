import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('seen_strings', (table) => {
    table.uuid('id').unique().primary().comment('Seen String ID')
    table.string('key', 1024).notNullable().comment('String Key')
    table.string('type', 255).notNullable()
    table.timestamp('created_at')
    table.timestamp('last_cached')
    table.unique(['key', 'type'])
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('seen_strings')
}
