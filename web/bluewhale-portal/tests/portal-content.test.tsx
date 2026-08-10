import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'

import { DocsPage } from '../src/components/docs-page'
import { HomePage } from '../src/components/home-page'
import { publicVideoScope } from '../src/lib/public-video-scope'

describe('BLUE WHALE public portal', () => {
  it('limits the landing page to Seedance 2.0 and the two public task operations', () => {
    const html = renderToStaticMarkup(<HomePage language='zh' />)

    expect(html).toContain('seedance-2.0')
    expect(html).toContain('把想象')
    expect(html).toContain('推入镜头')
    expect(html).toContain('生成画布')
    expect(html).toContain('提交任务')
    expect(html).toContain('查询任务')
    expect(html).not.toContain('generation_task')
    expect(html).not.toContain('queued')
    expect(html).not.toContain('curl')
    expect(html).not.toContain('Webhook')
    expect(html).not.toContain('GPT')
    expect(html).not.toContain('充值')
  })

  it('renders only the approved documentation navigation and endpoint paths', () => {
    const html = renderToStaticMarkup(<DocsPage language='zh' slug='quickstart' />)

    expect(publicVideoScope.model).toBe('seedance-2.0')
    expect(publicVideoScope.operations).toEqual([
      { method: 'POST', path: '/v1/video/generations' },
      { method: 'GET', path: '/v1/video/generations/{task_id}' }
    ])
    expect(html).toContain('/v1/video/generations')
    expect(html).toContain('/v1/video/generations/{task_id}')
    expect(html).not.toContain('Webhook')
    expect(html).not.toContain('Management API')
  })
})
