import MerryMaker from '@merrymaker/types'
import { Alert } from '../models'
import { AlertEvent, AlertSinkBase } from '../alerts/base'
import GoAlertSink from '../alerts/go-alert'
import KafkaAlertSink from '../alerts/kafka'
import logger from '../loaders/logger'

type MappedSinks = { [k in MerryMaker.ScanEventType]?: AlertSinkBase[] }

// knex supports `with`, but this is easier to maintain
const GENERATE_SERIES_SQL = `
with hours as (
  select generate_series(
    date_trunc('hour', to_timestamp(:starttime:)),
    date_trunc('hour', to_timestamp(:endtime:)),
    ':interval: hour':\\:interval
  ) as hour
)

select
  hours.hour,
  count(alerts)::integer
from hours
left join alerts on date_trunc('hour', alerts.created_at) = hours.hour
group by 1
order by hours.hour desc
`

/**
 * dateHist
 *
 * queries and returns a datehistogram of alerts
 * from `starttime` to `endtime` with an interval of `interval` hours
 */
const dateHist = async (opt: {
  starttime: Date
  endtime: Date
  interval: number
}): Promise<{ rows: Array<{ hour: string; count: number }> }> =>
  Alert.knex().raw(GENERATE_SERIES_SQL, {
    starttime: opt.starttime.valueOf() / 1000,
    endtime: opt.endtime.valueOf() / 1000,
    interval: opt.interval
  })


// Helper for mapping Alert sinks against ScanEventTypes
const alertSinks = {
  sinks: [] as MappedSinks,
  use: (sType: MerryMaker.ScanEventType, sink: AlertSinkBase) => {
    logger.info({
      task: 'alert loader',
      message: `loading "${sink.name}" for "${sType}"`,
    })
    if (Array.isArray(alertSinks.sinks[sType])) {
      alertSinks.sinks[sType].push(sink)
    } else {
      alertSinks.sinks[sType] = [sink]
    }
  },
}

if (GoAlertSink.enabled) {
  alertSinks.use('error', GoAlertSink)
  alertSinks.use('rule-alert', GoAlertSink)
}

if (KafkaAlertSink.enabled) {
  alertSinks.use('rule-alert', KafkaAlertSink)
}

const view = async (id: string): Promise<Alert> =>
  Alert.query().findById(id).throwIfNotFound()

const distinct = async (column: string): Promise<Alert[]> =>
  Alert.query().distinct(column)

const destroy = async (id: string): Promise<number> =>
  Alert.query().deleteById(id)

function isRuleAlert(
  evt: MerryMaker.EventResult | MerryMaker.RuleAlertEvent
): evt is MerryMaker.RuleAlertEvent {
  return evt.entry === 'rule-alert'
}

function toAlertEvent(evt: MerryMaker.EventResult): AlertEvent {
  let message: string
  let details: string
  if (evt.entry === 'error' || evt.entry === 'page-error') {
    message = (evt.event as MerryMaker.EventMessage).message
  } else if (isRuleAlert(evt)) {
    message = `${evt.event.name} - ${evt.event.message}`
    details = JSON.stringify(evt.event.context)
  } else {
    message = JSON.stringify(evt.event)
  }
  return {
    type: evt.level,
    name: evt.entry,
    scan_id: evt.scan_id,
    message,
    details,
    body: evt.event,
  }
}

export async function process(evt: MerryMaker.EventResult): Promise<void> {
  const alertEvent = toAlertEvent(evt)
  if (alertSinks.sinks[evt.entry] === undefined) return
  await Promise.all(
    alertSinks.sinks[evt.entry].map((s: AlertSinkBase) => s.send(alertEvent))
  )
}

export default {
  dateHist,
  distinct,
  process,
  destroy,
  view,
}
