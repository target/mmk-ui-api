import Chance from 'chance'
import ModelFactory from './base'
import { Site, SiteAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<Site, SiteAttributes>(
  {
    get name() {
      return chance.string({ length: 8, alpha: true, numeric: false })
    },
    active: true,
    run_every_minutes: 60,
    source_id: chance.guid({ version: 4 }),
  },
  Site
)
