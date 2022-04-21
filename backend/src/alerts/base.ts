/** Alert type Base */
import MerryMaker from '@merrymaker/types'

export interface AlertEvent {
  type: 'info' | 'error' | 'warning'
  name: string
  scan_id: string
  message: string
  details: string
  body?: object
}

export interface AlertSinkBase {
  name: string
  enabled: boolean
  send: (
    evt: MerryMaker.EventResult | MerryMaker.RuleAlertEvent | AlertEvent
  ) => Promise<boolean>
}
