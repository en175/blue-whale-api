import { describe, expect, it } from 'vitest'

import { metadata } from '../src/app/layout'

describe('discovery posture', () => {
  it('prevents indexing before an official domain is approved', () => {
    expect(metadata.robots).toMatchObject({
      follow: false,
      index: false
    })
  })
})
