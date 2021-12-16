import Chance from 'chance'
import ModelFactory from './base'
import { SeenString, SeenStringAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<SeenString, SeenStringAttributes>(
  {
    type: 'fqdn',
    get key() {
      return chance.domain()
    },
    created_at: new Date(),
    last_cached: new Date(),
  },
  SeenString
)
