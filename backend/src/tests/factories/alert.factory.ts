import Chance from 'chance'
import ModelFactory from './base'
import { Alert, AlertAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<Alert, AlertAttributes>(
  {
    rule: 'unknown.domain',
    message: 'example.com unknown',
    context: { url: 'https://example.com/script.js' },
    scan_id: chance.guid({ version: 4 }),
    site_id: chance.guid({ version: 4 }),
    created_at: new Date(),
  },
  Alert
)
