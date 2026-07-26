import assert from 'node:assert/strict'
import test from 'node:test'

import { tunerDetails, type TunerInfo } from './tunerStatus.ts'

test('tunerDetails explains viewer usage and physical sharing', () => {
  const tuner: TunerInfo = {
    type: 'GR', total: 4, using: 4, free: 0,
    viewerUsing: 1, epgUsing: 3, otherUsing: 1, viewerShared: 1,
  }
  assert.equal(
    tunerDetails(tuner),
    'TV Viewer 1台（うち共有1台） / EPG取得 3台 / その他 1台 / 空き 0台',
  )
})
