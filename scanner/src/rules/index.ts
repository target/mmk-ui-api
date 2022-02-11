import ScanEventHandler from '../lib/scan-event-handler'
import unknownDomainRule from './unknown-domain'
import iocDomainRule from './ioc.domain'
import iocPayloadRule from './ioc.payload'
import yaraRule from './yara'
import webSocketRule from './websocket'
import googleAnalyticsRule from './google-analytics'
import htmlSnapshot from './html-snapshot'

const scanHandler = new ScanEventHandler()

// Rules
scanHandler.use('request', unknownDomainRule)
scanHandler.use('request', iocDomainRule)
scanHandler.use('request', iocPayloadRule)
scanHandler.use('request', googleAnalyticsRule)
scanHandler.use('script-response', yaraRule)
scanHandler.use('function-call', webSocketRule)
scanHandler.use('html-snapshot', htmlSnapshot)

export { scanHandler }
