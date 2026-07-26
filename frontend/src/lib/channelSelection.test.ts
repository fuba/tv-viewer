import test from 'node:test'
import assert from 'node:assert/strict'

import { channelFromEPG, channelSelectionId, resolveActiveChannel } from './channelSelection.ts'

test('resolveActiveChannel selects the exact service on a shared physical channel', () => {
  const channels = [{
    type: 'CS',
    channel: 'CS6',
    services: [
      { id: 700294, name: 'Home Drama' },
      { id: 700354, name: 'CNNj' }
    ]
  }]

  const selected = resolveActiveChannel(channels, 'service:CS:CS6:700354')

  assert.equal(selected?.serviceName, 'CNNj')
  assert.equal(channelSelectionId(selected), 'service:CS:CS6:700354')
})

test('channelFromEPG creates a service-scoped channel selection', () => {
  assert.deepEqual(channelFromEPG({
    channel: 'CS16', type: 'CS', serviceId: 353, name: 'BBCニュース',
  }), {
    channel: 'CS16', type: 'CS', serviceId: 353, name: 'BBCニュース',
    displayName: 'BBCニュース', isService: true,
    services: [{ id: 353, serviceId: 353, name: 'BBCニュース' }],
  })
})
