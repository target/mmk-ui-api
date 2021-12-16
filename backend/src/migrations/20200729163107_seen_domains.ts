import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('seen_domains', (table) => {
    table
      .string('name', 1024)
      .notNullable()
      .unique()
      .primary()
      .comment('Domain name')
    table.timestamp('created_at')
    table.uuid('site_id').references('id').inTable('sites')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('seen_domains')
}
