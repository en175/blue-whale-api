import { describe, expect, it } from 'vitest'

import { documentationPages } from '../src/lib/docs-content'
import { findForbiddenPublicContent } from '../scripts/check-public-surface'

describe('public surface guard', () => {
  it('accepts the approved Seedance 2.0 task scope', () => {
    expect(
      findForbiddenPublicContent(
        'seedance-2.0 POST /v1/video/generations GET /v1/video/generations/{task_id}'
      )
    ).toEqual([])
  })

  it('keeps every documentation page within the public surface', () => {
    expect(findForbiddenPublicContent(JSON.stringify(documentationPages))).toEqual([])
    expect(
      documentationPages.flatMap((page) => page.sections).filter((section) => Object.hasOwn(section, 'code'))
    ).toEqual([])
  })

  it('rejects unpublished features, generic gateway marketing, and prototype addresses', () => {
    expect(
      findForbiddenPublicContent(
        'Webhook GPT-4 管理 API 充值 http://115.190.244.91:3005'
      )
    ).toEqual(
      expect.arrayContaining(['Webhook', 'GPT-4', '管理 API', '充值', '115.190.244.91'])
    )
  })

  it('rejects case variants, unapproved authentication details, and non-public API paths', () => {
    expect(
      findForbiddenPublicContent(
        'webhook CALLBACK Idempotency-Key Authorization Bearer Token /v1/chat/completions'
      )
    ).toEqual(
      expect.arrayContaining([
        'Webhook',
        'callback',
        'idempotency',
        'Authorization',
        'Bearer Token',
        '非公开 API 路径: /v1/chat/completions'
      ])
    )
  })
})
