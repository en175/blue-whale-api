/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { LogOtherData } from '../types'
import { getLogGroupRatioLabel, resolveLogGroupRatio } from './format'

// 展示层必须和后端 ResolveGroupRatio 用同一套优先级：命中哪一层就显示哪一层的倍率。
// 重点在「短路」——模型折扣和用户组特价可能同时非空，只有优先级高的那个是实收值。
describe('resolveLogGroupRatio', () => {
  test('模型折扣优先于用户组特价与分组倍率', () => {
    const other: LogOtherData = {
      group_ratio: 0.4,
      user_group_ratio: 0.5,
      model_group_ratio: 0.4,
    }
    assert.deepEqual(resolveLogGroupRatio(other), {
      ratio: 0.4,
      source: 'model',
    })
  })

  test('模型折扣配 1 时不能被用户组特价 0.5 顶掉', () => {
    // 实收 1，展示 0.5 就是错的；这正是「谁不为空用谁」会踩的坑
    const other: LogOtherData = {
      group_ratio: 1,
      user_group_ratio: 0.5,
      model_group_ratio: 1,
    }
    assert.deepEqual(resolveLogGroupRatio(other), { ratio: 1, source: 'model' })
  })

  test('模型折扣配 0 时按 0 展示，不能被当成未配置', () => {
    const other: LogOtherData = {
      group_ratio: 0.8,
      user_group_ratio: 0.5,
      model_group_ratio: 0,
    }
    assert.deepEqual(resolveLogGroupRatio(other), { ratio: 0, source: 'model' })
  })

  test('未命中模型层时回落到用户组特价', () => {
    const other: LogOtherData = { group_ratio: 0.5, user_group_ratio: 0.5 }
    assert.deepEqual(resolveLogGroupRatio(other), {
      ratio: 0.5,
      source: 'user',
    })
  })

  test('-1 是未配置哨兵，两层都跳过', () => {
    const other: LogOtherData = {
      group_ratio: 0.8,
      user_group_ratio: -1,
      model_group_ratio: -1,
    }
    assert.deepEqual(resolveLogGroupRatio(other), {
      ratio: 0.8,
      source: 'group',
    })
  })

  test('非有限数不参与展示', () => {
    const other: LogOtherData = {
      group_ratio: 0.8,
      user_group_ratio: Number.NaN,
      model_group_ratio: Number.POSITIVE_INFINITY,
    }
    assert.deepEqual(resolveLogGroupRatio(other), {
      ratio: 0.8,
      source: 'group',
    })
  })

  test('没有 other 时返回空倍率且来源为分组层', () => {
    assert.deepEqual(resolveLogGroupRatio(null), {
      ratio: null,
      source: 'group',
    })
  })
})

describe('getLogGroupRatioLabel', () => {
  const t = (key: string) => key

  test('按来源层给出对应文案 key', () => {
    assert.equal(getLogGroupRatioLabel('model', t), 'Model Group Ratio')
    assert.equal(getLogGroupRatioLabel('user', t), 'User Exclusive Ratio')
    assert.equal(getLogGroupRatioLabel('group', t), 'Group Ratio')
  })
})
