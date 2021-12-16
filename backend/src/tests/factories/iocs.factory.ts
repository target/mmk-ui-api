import Chance from 'chance'
import ModelFactory from './base'
import { Ioc, IocAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<Ioc, IocAttributes>(
  {
    type: 'fqdn',
    get value() {
      return chance.domain()
    },
    enabled: true,
    created_at: new Date(),
  },
  Ioc
)
