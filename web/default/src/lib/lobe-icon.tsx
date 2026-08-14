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
/**
 * LobeHub Icon Loader
 * Dynamically load and render icons from @lobehub/icons
 *
 * Supports:
 * - Basic: "OpenAI", "OpenAI.Color"
 * - Chained properties: "OpenAI.Avatar.type={'platform'}"
 * - Size parameter: getLobeIcon("OpenAI", 20)
 */
import { useState } from 'react'

import * as LobeIcons from '@lobehub/icons'

/**
 * TOS-hosted colored brand logos.
 * When the icon name matches a known brand, use the colored logo from TOS
 * instead of the monochrome React component.
 */
const TOS_LOGO_BASE =
  'https://bluewhale-assets.tos-cn-beijing.volces.com/blue-whale-api/homepage/brand-logos'

const BRAND_LOGO_MAP: Record<string, string> = {
  openai: `${TOS_LOGO_BASE}/openai.png`,
  claude: `${TOS_LOGO_BASE}/claude.png`,
  anthropic: `${TOS_LOGO_BASE}/claude.png`,
  gemini: `${TOS_LOGO_BASE}/gemini.png`,
  google: `${TOS_LOGO_BASE}/gemini.png`,
  deepseek: `${TOS_LOGO_BASE}/deepseek.png`,
  qwen: `${TOS_LOGO_BASE}/qwen.png`,
  tongyi: `${TOS_LOGO_BASE}/qwen.png`,
  doubao: `${TOS_LOGO_BASE}/doubao.png`,
  bytedance: `${TOS_LOGO_BASE}/doubao.png`,
  moonshot: `${TOS_LOGO_BASE}/moonshot.png`,
  kimi: `${TOS_LOGO_BASE}/moonshot.png`,
  kling: `${TOS_LOGO_BASE}/kling.png`,
  kuaishou: `${TOS_LOGO_BASE}/kling.png`,
  zhipu: `${TOS_LOGO_BASE}/zhipu.png`,
  glm: `${TOS_LOGO_BASE}/zhipu.png`,
  chatglm: `${TOS_LOGO_BASE}/zhipu.png`,
  minimax: `${TOS_LOGO_BASE}/minimax.png`,
  yi: `${TOS_LOGO_BASE}/yi.webp`,
  hunyuan: `${TOS_LOGO_BASE}/hunyuan.webp`,
  tencent: `${TOS_LOGO_BASE}/hunyuan.webp`,
  xfai: '/xfai.png',
}

/**
 * Try to resolve a TOS colored logo URL from an icon name.
 * Returns the URL if found, undefined otherwise.
 */
function resolveTosLogo(iconName: string): string | undefined {
  const key = iconName.toLowerCase().trim()
  return BRAND_LOGO_MAP[key]
}

/**
 * Parse a property value from string to appropriate type
 * @param raw - Raw string value
 * @returns Parsed value (boolean, number, or string)
 */
function parseValue(raw: string | undefined | null): string | number | boolean {
  if (raw == null) return true

  let v = String(raw).trim()

  // Remove curly braces
  if (v.startsWith('{') && v.endsWith('}')) {
    v = v.slice(1, -1).trim()
  }

  // Remove quotes
  if (
    (v.startsWith('"') && v.endsWith('"')) ||
    (v.startsWith("'") && v.endsWith("'"))
  ) {
    return v.slice(1, -1)
  }

  // Boolean
  if (v === 'true') return true
  if (v === 'false') return false

  // Number
  if (/^-?\d+(?:\.\d+)?$/.test(v)) return Number(v)

  // Return as string
  return v
}

/**
 * Renders a TOS brand logo image, falling back to the lobehub component
 * when the remote asset fails to load.
 */
function BrandLogoImage(props: {
  src: string
  alt: string
  size: number
  fallback: React.ReactNode
}) {
  const [failed, setFailed] = useState(false)
  if (failed) return <>{props.fallback}</>
  return (
    <img
      src={props.src}
      alt={props.alt}
      width={props.size}
      height={props.size}
      style={{
        width: props.size,
        height: props.size,
        objectFit: 'contain',
        flexShrink: 0,
      }}
      loading='lazy'
      onError={() => setFailed(true)}
    />
  )
}

/**
 * Resolve and render a lobehub icon component from a dotted icon name.
 */
function renderLobeComponent(
  trimmedName: string,
  size: number
): React.ReactNode {
  // Parse component path and chained properties
  const segments = trimmedName.split('.')
  const baseKey = segments[0]
  const BaseIcon = (LobeIcons as Record<string, unknown>)[baseKey] as
    | Record<string, unknown>
    | undefined

  let IconComponent: React.ComponentType<Record<string, unknown>> | undefined
  let propStartIndex: number

  if (BaseIcon && segments.length > 1 && BaseIcon[segments[1]]) {
    IconComponent = BaseIcon[segments[1]] as React.ComponentType<
      Record<string, unknown>
    >
    propStartIndex = 2
  } else {
    IconComponent = (LobeIcons as Record<string, unknown>)[baseKey] as
      | React.ComponentType<Record<string, unknown>>
      | undefined
    propStartIndex = segments.length > 1 && /^[A-Z]/.test(segments[1]) ? 2 : 1
  }

  // Fallback if icon not found
  if (
    !IconComponent ||
    (typeof IconComponent !== 'function' && typeof IconComponent !== 'object')
  ) {
    const firstLetter = trimmedName.charAt(0).toUpperCase()
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        {firstLetter}
      </div>
    )
  }

  // Parse chained properties (e.g., "type={'platform'}", "shape='square'")
  const props: Record<string, string | number | boolean> = {}

  for (let i = propStartIndex; i < segments.length; i++) {
    const seg = segments[i]
    if (!seg) continue

    const eqIdx = seg.indexOf('=')
    if (eqIdx === -1) {
      props[seg.trim()] = true
      continue
    }

    const key = seg.slice(0, eqIdx).trim()
    const valRaw = seg.slice(eqIdx + 1).trim()
    props[key] = parseValue(valRaw)
  }

  // Set size if not explicitly specified in the string
  if (props.size == null && size != null) {
    props.size = size
  }

  return <IconComponent {...props} />
}

/**
 * Get LobeHub icon component by name
 * @param iconName - Icon name/description (e.g., "OpenAI", "OpenAI.Color", "Claude.Avatar")
 * @param size - Icon size (default: 20)
 * @returns Icon component or fallback
 *
 * @example
 * getLobeIcon("OpenAI", 24)
 * getLobeIcon("OpenAI.Color", 20)
 * getLobeIcon("Claude.Avatar.type={'platform'}", 32)
 */
export function getLobeIcon(
  iconName: string | undefined | null,
  size: number = 20
): React.ReactNode {
  if (!iconName || typeof iconName !== 'string') {
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        ?
      </div>
    )
  }

  const trimmedName = iconName.trim()
  if (!trimmedName) {
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        ?
      </div>
    )
  }

  // Prefer TOS colored brand logos for known vendors; fall back to the
  // lobehub component if the remote asset fails to load.
  const baseName = trimmedName.split('.')[0]
  const tosLogo = resolveTosLogo(baseName)
  if (tosLogo) {
    return (
      <BrandLogoImage
        src={tosLogo}
        alt={baseName}
        size={size}
        fallback={renderLobeComponent(trimmedName, size)}
      />
    )
  }

  return renderLobeComponent(trimmedName, size)
}
