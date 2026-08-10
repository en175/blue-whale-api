import { describe, expect, it } from 'vitest'

import LocalePage from '../src/app/[lang]/page'

describe('public locale route', () => {
  it('renders the published Chinese route', async () => {
    await expect(LocalePage({ params: Promise.resolve({ lang: 'zh' }) })).resolves.toBeDefined()
  })

  it('rejects every language route that is not published', async () => {
    await expect(LocalePage({ params: Promise.resolve({ lang: 'en' }) })).rejects.toThrow()
    await expect(LocalePage({ params: Promise.resolve({ lang: 'anything' }) })).rejects.toThrow()
  })
})
