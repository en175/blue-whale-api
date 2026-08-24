import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, Plus, Upload, UserRound } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { getMaterialGroups } from './api'
import { MaterialAssetTable } from './components/asset-table'
import { CreateGroupDialog } from './components/create-group-dialog'
import { MaterialGroupList } from './components/group-list'
import { UploadAssetDialog } from './components/upload-asset-dialog'
import type { MaterialGroupType } from './types'

/**
 * 素材中心：顶部真人素材/AIGC tab 切换类型；
 * 左侧列出用户有权限的素材组（本地归属表，数据隔离），支持新建/删除；
 * 选中素材组后右侧按组 id 查上游素材（分页表格），支持上传/重命名/删除。
 */
export function Materials() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [groupType, setGroupType] = useState<MaterialGroupType>('AIGC')
  const [selectedGroupId, setSelectedGroupId] = useState<string>('')
  const [search, setSearch] = useState('')
  const [createGroupOpen, setCreateGroupOpen] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [page, setPage] = useState(1)
  const pageSize = 10

  const { data, isLoading: groupsLoading } = useQuery({
    queryKey: ['material-groups', groupType, search, page, pageSize],
    queryFn: () => getMaterialGroups(groupType, page, pageSize, search),
  })
  const groups = data?.items ?? []
  const totalCount = data?.total_count ?? 0
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize))

  // 切换 tab 或组列表变化时，默认选中第一个素材组，避免右侧空白。
  useEffect(() => {
    if (!selectedGroupId && groups.length > 0) {
      setSelectedGroupId(groups[0].seedance_asset_group_id)
    }
  }, [groups, selectedGroupId])

  // 删除后若当前页超出总页数（例如删掉最后一页的唯一组），回退到最后一页，避免停留空页。
  useEffect(() => {
    if (totalCount > 0 && page > totalPages) {
      setPage(totalPages)
      setSelectedGroupId('')
    }
  }, [totalCount, page, totalPages])

  const handleTabChange = (value: string) => {
    setGroupType(value as MaterialGroupType)
    setSelectedGroupId('')
    setSearch('')
    setPage(1)
  }

  const handleSearchChange = (value: string) => {
    setSearch(value)
    setSelectedGroupId('')
    setPage(1) // 搜索只作用于当前页会误导，重置到第 1 页配合后端过滤
  }

  const handlePageChange = (nextPage: number) => {
    // 边界防护：钳制到 [1, totalPages]，非法输入（0/负数/超界）不生效。
    const clamped = Math.min(Math.max(1, nextPage), totalPages)
    setPage(clamped)
    setSelectedGroupId('') // 翻页后不保留旧页选中组
  }

  // 前缀失效：覆盖 search/page 全变体（建组/删组影响所有过滤视图，不单是当前页）。
  const refreshGroups = () =>
    queryClient.invalidateQueries({
      queryKey: ['material-groups', groupType],
    })

  const refreshAssets = () => {
    if (selectedGroupId) {
      queryClient.invalidateQueries({
        queryKey: ['material-assets', selectedGroupId],
      })
    }
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Materials')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button onClick={() => setCreateGroupOpen(true)}>
            <Plus className='size-4' />
            {t('New Material Group')}
          </Button>
          <Button
            onClick={() => setUploadOpen(true)}
            disabled={!selectedGroupId}
            variant='outline'
          >
            <Upload className='size-4' />
            {t('Upload Asset')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs value={groupType} onValueChange={handleTabChange}>
            <TabsList className='h-auto w-full gap-1 rounded-xl p-1 sm:w-auto'>
              <TabsTrigger
                value='LivenessFace'
                className='flex-1 gap-2 rounded-lg px-4 py-2.5 data-active:shadow-sm sm:flex-none'
              >
                <UserRound className='size-4' />
                {t('Real Person Materials')}
              </TabsTrigger>
              <TabsTrigger
                value='AIGC'
                className='flex-1 gap-2 rounded-lg px-4 py-2.5 data-active:shadow-sm sm:flex-none'
              >
                <Bot className='size-4' />
                {t('AIGC Materials')}
              </TabsTrigger>
            </TabsList>
          </Tabs>

          <div className='mt-4 flex flex-col gap-4 md:flex-row'>
            <div className='w-full shrink-0 md:w-72'>
              <MaterialGroupList
                groups={groups}
                isLoading={groupsLoading}
                groupType={groupType}
                selectedGroupId={selectedGroupId}
                onSelect={setSelectedGroupId}
                totalCount={totalCount}
                page={page}
                pageSize={pageSize}
                onPageChange={handlePageChange}
                search={search}
                onSearchChange={handleSearchChange}
              />
            </div>
            <div className='min-w-0 flex-1'>
              <MaterialAssetTable groupId={selectedGroupId} />
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {/* 对话框必须是 SectionPageLayout 的兄弟节点：该布局只渲染四个槽，忽略其它子节点 */}
      <CreateGroupDialog
        open={createGroupOpen}
        onOpenChange={setCreateGroupOpen}
        defaultGroupType={groupType}
        onCreated={refreshGroups}
      />
      <UploadAssetDialog
        groupId={selectedGroupId}
        open={uploadOpen}
        onOpenChange={setUploadOpen}
        onUploaded={refreshAssets}
      />
    </>
  )
}
