import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.table('alerts', (table) => {
    table.jsonb('context').nullable().comment('Alert Context')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.table('alerts', (table) => {
    table.dropColumn('context')
  })
}
