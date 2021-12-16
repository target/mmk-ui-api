import { QueryBuilder } from 'objection'
import Secret from '../../../models/secrets'
import Scan from '../../../models/scans'
import Source from '../../../models/sources'

export const eagerLoad = (
  eagers: string[],
  builder: QueryBuilder<Source>
): void => {
  if (eagers.includes('scans')) {
    builder.withGraphFetched('scans(selectID)').modifiers({
      selectID(builder: QueryBuilder<Scan>) {
        builder.select('id')
      },
    })
  }
  // eager load secrets
  if (eagers.includes('secrets')) {
    builder.withGraphFetched('secrets(selectSecret)').modifiers({
      selectSecret(builder: QueryBuilder<Secret>) {
        builder.select('id', 'name', 'type')
      },
    })
  }
}
