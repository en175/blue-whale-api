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
import { useState, useCallback } from 'react'
import { toast } from 'sonner'

import { requestBlueWhalePayment, isApiSuccess } from '../api'

function getPayData(data: unknown): string | null {
  if (!data || typeof data !== 'object') return null
  const d = data as Record<string, unknown>
  if ('pay_data' in d && typeof d.pay_data === 'string') return d.pay_data
  return null
}

function getErrorMessage(message: string | undefined, data: unknown): string {
  if (typeof data === 'string' && data.trim()) return data
  return message || '支付请求失败'
}

export type BlueWhalePayOrderInfo = {
  orderId: string
  createdAt: number
}

/**
 * blue-whale-pay 支付 hook（支付宝 ALI_QR / 微信 WX_NATIVE）。
 *
 * PC 端统一二维码：pay 返二维码内容，前端 QRCodeSVG 弹窗展示。
 * loadingWayCode 区分哪个方式在 loading（只点击的按钮 spinner + "订单生成中"）。
 * qrCodeWayCode 记当前二维码是哪种支付（弹窗标题/图标区分）。
 * orderInfo 记订单号 + 创建时间（弹窗展示订单号 + 超时倒计时）。
 */
export function useBlueWhalePayPayment() {
  const [loadingWayCode, setLoadingWayCode] = useState<string | null>(null)
  const [qrCode, setQrCode] = useState<string | null>(null)
  const [qrCodeWayCode, setQrCodeWayCode] = useState<string | null>(null)
  const [orderInfo, setOrderInfo] = useState<BlueWhalePayOrderInfo | null>(null)

  const processBlueWhalePayPayment = useCallback(
    async (topupAmount: number, wayCode: string) => {
      setLoadingWayCode(wayCode)
      try {
        const response = await requestBlueWhalePayment({
          amount: Math.floor(topupAmount),
          way_code: wayCode,
        })

        if (isApiSuccess(response)) {
          const payData = getPayData(response.data)
          if (payData) {
            const orderId =
              (response.data as { order_id?: string } | null)?.order_id || ''
            setQrCode(payData)
            setQrCodeWayCode(wayCode)
            setOrderInfo({ orderId, createdAt: Date.now() })
            return true
          }
        }

        toast.error(getErrorMessage(response.message, response.data))
        return false
      } catch (_error) {
        toast.error('支付请求失败')
        return false
      } finally {
        setLoadingWayCode(null)
      }
    },
    []
  )

  const closeQrCode = useCallback(() => {
    setQrCode(null)
    setQrCodeWayCode(null)
    setOrderInfo(null)
  }, [])

  return {
    loadingWayCode,
    processBlueWhalePayPayment,
    qrCode,
    closeQrCode,
    qrCodeWayCode,
    orderInfo,
  }
}
