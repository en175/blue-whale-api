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
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from '@tanstack/react-router'
import {
  CoinsIcon,
  LayoutDashboardIcon,
  LayersIcon,
  ReceiptIcon,
  SettingsIcon,
  UsersIcon,
} from 'lucide-react'

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

import { ADMIN_NAV_ITEMS } from '../lib/constants'

const ICON_MAP: Record<string, React.ElementType> = {
  'layout-dashboard': LayoutDashboardIcon,
  layers: LayersIcon,
  coins: CoinsIcon,
  users: UsersIcon,
  receipt: ReceiptIcon,
  settings: SettingsIcon,
}

export function AdminSidebar() {
  const { t } = useTranslation()
  const href = useLocation({ select: (l) => l.pathname })

  return (
    <Sidebar collapsible="icon" variant="inset" className="border-r">
      <SidebarContent className="py-2">
        <SidebarGroup className="px-2 py-1">
          <SidebarGroupLabel className="text-muted-foreground/70 px-2 text-[11px] font-medium tracking-wider uppercase">
            Admin
          </SidebarGroupLabel>
          <SidebarMenu>
            {ADMIN_NAV_ITEMS.map((item) => {
              const Icon = ICON_MAP[item.icon]
              const isActive =
                item.to === '/admin'
                  ? href === '/admin'
                  : href.startsWith(item.to)

              return (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    isActive={isActive}
                    tooltip={t(item.title)}
                    className={cn(isActive && 'bg-sidebar-accent')}
                    render={<Link to={item.to as any} />}
                  >
                    {Icon && <Icon className="size-4 shrink-0" />}
                    <span className="min-w-0 flex-1 truncate">
                      {t(item.title)}
                    </span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )
            })}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}
