import { queryFromAlert } from '../alerts/go-alert'

describe('Go Alert', function () {
  describe('queryFromAlert', function () {
    it('formats the query string', (done) => {
      const actual = queryFromAlert(
        {
          name: 'example.name',
          message: 'example message',
          details: 'example details with extra details',
          type: 'info',
          scan_id: '12345',
        },
        'example-token'
      )
      expect(actual).toEqual(
        'summary=example.name%20-%20example%20message&details=example%20details%20with%20extra%20details&token=example-token'
      )
      done()
    })
  })
})
