import Chance from 'chance'
import ModelFactory from './base'
import { AllowList, AllowListAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<AllowList, AllowListAttributes>(
  {
    type: 'fqdn',
    get key() {
      return chance.domain()
    },
    created_at: new Date(),
    updated_at: new Date(),
  },
  AllowList
)
