export type MaterialGroupType = 'AIGC' | 'LivenessFace'

/** 本地归属表中的素材组（管理界面左侧列表数据源）。 */
export interface MaterialGroup {
  id: number
  user_id: number
  seedance_asset_group_id: string
  name: string
  group_type: MaterialGroupType
  created_at: number
  updated_at: number
}

/** 上游 ARK 接口的素材对象（右侧表格数据源）。 */
export interface MaterialAsset {
  Id: string
  Name: string
  URL: string
  GroupId: string
  AssetType: string
  Status: string
  CreateTime: string
  UpdateTime: string
}

/** 上游 ListAssets 的 Result。 */
export interface ListAssetsResult {
  TotalCount: number
  Items: MaterialAsset[]
  PageNumber: number
  PageSize: number
}
