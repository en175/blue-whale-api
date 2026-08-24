import { api } from '@/lib/api'

import type {
  ListAssetsResult,
  MaterialGroup,
  MaterialGroupType,
} from './types'

// ============================================================================
// 素材库管理界面 API（UserAuth，与 TokenAuth 的 /api/material/* 代理分开）
// ============================================================================

/** 查询当前用户有权限的素材组（本地归属表，普通用户隔离、admin 看全部）。分页返回；name 按组名/组 id 模糊匹配。 */
export async function getMaterialGroups(
  groupType?: MaterialGroupType,
  page = 1,
  pageSize = 20,
  name = ''
): Promise<{
  total_count: number
  items: MaterialGroup[]
  page_number: number
  page_size: number
}> {
  const res = await api.get('/api/materials/groups', {
    params: {
      ...(groupType ? { type: groupType } : {}),
      p: page,
      page_size: pageSize,
      ...(name ? { name } : {}),
    },
  })
  return (
    res.data?.data ?? {
      total_count: 0,
      items: [],
      page_number: page,
      page_size: pageSize,
    }
  )
}

/** 创建素材组（上游 CreateAssetGroup），成功后在本地记录归属。 */
export async function createMaterialGroup(data: {
  name: string
  description?: string
  group_type: MaterialGroupType
}): Promise<{ id: string }> {
  const res = await api.post('/api/materials/groups', data)
  return res.data?.data
}

/**
 * 发起真人素材认证会话（UserAuth 管理端点，服务端内置凭证直连上游）。
 * 回调指向本网关 /v1/material/h5-callback（H5 认证完成后浏览器跳回，网关后台轮询拿 GroupId）。
 * name 为用户起的真人组名，认证完成后用作归属组名。
 */
export async function createVisualValidateSession(
  name: string
): Promise<{
  h5_link: string
  byted_token: string
}> {
  const res = await api.post('/api/materials/visual-sessions', {
    name,
    callback_url: `${window.location.origin}/v1/material/h5-callback`,
    project_name: 'default',
  })
  return res.data?.data ?? {}
}

/** 删除素材组（上游 DeleteAssetGroup），同步清理本地归属。 */
export async function deleteMaterialGroup(
  groupId: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(`/api/materials/groups/${groupId}`)
  return res.data
}

/** 按素材组 id 查询上游素材（分页 + 名称模糊匹配透传上游）。 */
export async function getGroupAssets(
  groupId: string,
  page: number,
  pageSize: number,
  name?: string
): Promise<ListAssetsResult> {
  const res = await api.get(`/api/materials/groups/${groupId}/assets`, {
    params: { p: page, page_size: pageSize, name: name || undefined },
  })
  return res.data?.data
}

/** 上传素材（上游 CreateAsset，URL 直传）。 */
export async function createMaterialAsset(
  groupId: string,
  data: { url: string; name: string; asset_type: string }
): Promise<{ id: string }> {
  const res = await api.post(`/api/materials/groups/${groupId}/assets`, data)
  return res.data?.data
}

/** 重命名素材（上游 UpdateAsset）。 */
export async function renameMaterialAsset(
  groupId: string,
  assetId: string,
  name: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.put(
    `/api/materials/groups/${groupId}/assets/${assetId}`,
    { name }
  )
  return res.data
}

/** 删除素材（上游 DeleteAsset）。 */
export async function deleteMaterialAsset(
  groupId: string,
  assetId: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(
    `/api/materials/groups/${groupId}/assets/${assetId}`
  )
  return res.data
}
