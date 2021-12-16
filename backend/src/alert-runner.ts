import MerryMaker from '@merrymaker/types'
import AlertService from './services/alert'

;(async () => {
  // error
  await AlertService.process({
    level: 'error',
    entry: 'error',
    scan_id: '000-000',
    event: {
      message: 'test error message',
    },
  })
  // rule-alert
  await AlertService.process({
    level: 'info',
    entry: 'rule-alert',
    rule: 'test.rule',
    scan_id: '000-0000',
    event: {
      alert: true,
      name: 'test.rule',
      error: false,
      level: 'test',
      description: 'test rule alert - ignore',
      playbook: 'http://test/playbook',
      context: { key: 'value' },
      message: 'rule alert',
    } as MerryMaker.RuleAlert,
  } as MerryMaker.RuleAlertEvent)
})()
