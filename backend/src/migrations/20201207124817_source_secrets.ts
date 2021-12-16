import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('source_secrets', (table) => {
    table
      .uuid('secret_id')
      .references('id')
      .inTable('secrets')
      .notNullable()
      .onDelete('CASCADE')
    table
      .uuid('source_id')
      .references('id')
      .inTable('sources')
      .notNullable()
      .onDelete('CASCADE')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('source_secrets')
}
