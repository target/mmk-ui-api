import { QueryBuilder } from 'objection'
import { Source, Site, Scan } from '../../../models'

export const eagerLoad = (
  eagers: string[],
  builder: QueryBuilder<Scan>
): void => {
  if (eagers.includes('sources')) {
    builder.withGraphFetched('source(selectName)').modifiers({
      // only include the name
      selectName(builder: QueryBuilder<Source>) {
        builder.select('name')
      },
    })
  }
  if (eagers.includes('sites')) {
    builder.withGraphFetched('site(selectName)').modifiers({
      // only include the name
      selectName(builder: QueryBuilder<Site>) {
        builder.select('name')
      },
    })
  }
}
