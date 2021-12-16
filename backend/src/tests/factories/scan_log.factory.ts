import Chance from 'chance'
import ModelFactory from './base'
import { ScanLog, ScanLogAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<ScanLog, ScanLogAttributes>(
  {
    entry: 'complete',
    event: { message: 'completed scan' },
    get scan_id() {
      return chance.guid()
    },
    level: 'info',
    created_at: new Date(),
  },
  ScanLog
)
