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
import { Outlet } from '@tanstack/react-router'

import { AppHeader } from '@/components/layout/components/app-header'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'

import { AdminSidebar } from './admin-sidebar'

export function AdminLayout() {
  return (
    <SidebarProvider defaultOpen className="flex-col">
      <SkipToMain />
      <AppHeader
        showTopNav={false}
        showSearch={false}
        showNotifications={false}
        showConfigDrawer={false}
      />
      <div className="flex min-h-0 w-full flex-1">
        <AdminSidebar />
        <SidebarInset className="h-[calc(100svh-var(--app-header-height,0px))] min-h-0 overflow-auto">
          <div className="p-6">
            <Outlet />
          </div>
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}
