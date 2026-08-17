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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

interface AvailableModel {
  model_name: string
  display_name: string
  description: string
  param_schema: string
  sort_order: number
}

async function getUserDocsModels(): Promise<AvailableModel[]> {
  const res = await api.get('/api/user/docs-models')
  const data = res.data
  return Array.isArray(data?.data) ? data.data : []
}

export function AvailableModels() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['user-docs-models'],
    queryFn: getUserDocsModels,
  })

  if (isLoading) return null
  if (!data?.length) return null

  return (
    <div className='flex flex-col gap-2'>
      <h3 className='text-sm font-medium'>{t('Available Models')}</h3>
      <div className='flex flex-wrap gap-2'>
        {data.map((m) => (
          <div
            key={m.model_name}
            className='border-border/60 bg-muted/25 rounded-md border px-3 py-2 text-sm'
          >
            <span className='font-medium'>{m.display_name || m.model_name}</span>
            {m.description ? (
              <p className='text-muted-foreground text-xs'>{m.description}</p>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  )
}
