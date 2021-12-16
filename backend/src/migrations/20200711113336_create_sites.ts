import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('sites', (table) => {
    table
      .uuid('id')
      .notNullable()
      .unique()
      .primary()
      .comment('Primary key (uuid)')
    table.string('name').notNullable().unique().comment('Name of site')
    table.dateTime('last_run').comment('Last run time')
    table.boolean('active').defaultTo(true).comment('Site is actively running')
    table.integer('run_every_minutes').defaultTo(60).notNullable()
    table.uuid('source_id').references('sources.id')
    table.timestamps(true, true)
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('sites')
}
