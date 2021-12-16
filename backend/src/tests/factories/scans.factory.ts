import Chance from 'chance'
import ModelFactory from './base'
import { Scan, ScanAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<Scan, ScanAttributes>(
  {
    site_id: chance.guid({ version: 4 }),
    source_id: chance.guid({ version: 4 }),
    state: 'scheduled',
    created_at: new Date(),
  },
  Scan
)
