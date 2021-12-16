import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('allow_list', (table) => {
    table.uuid('id').unique().primary().comment('Allow list ID')
    table.string('key', 1024).notNullable()
    table.string('type', 255).notNullable()
    table.timestamp('created_at')
    table.timestamp('updated_at')
    table.unique(['key', 'type'])
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('allow_list')
}
