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
import { Gift, ExternalLink, Loader2, Receipt, WalletCards } from 'lucide-react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  formatCurrency,
  getDiscountLabel,
  getPaymentIcon,
  getMinTopupAmount,
  calculatePresetPricing,
} from '../lib'
import type {
  PaymentMethod,
  PresetAmount,
  TopupInfo,
  CreemProduct,
  WaffoPayMethod,
} from '../types'
import { CreemProductsSection } from './creem-products-section'
import { SiAlipay, SiWechat } from 'react-icons/si'
import { QRCodeSVG } from 'qrcode.react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { BlueWhalePayOrderInfo } from '../hooks'

interface RechargeFormCardProps {
  topupInfo: TopupInfo | null
  presetAmounts: PresetAmount[]
  selectedPreset: number | null
  onSelectPreset: (preset: PresetAmount) => void
  topupAmount: number
  onTopupAmountChange: (amount: number) => void
  paymentAmount: number
  calculating: boolean
  onPaymentMethodSelect: (method: PaymentMethod) => void
  paymentLoading: string | null
  redemptionCode: string
  onRedemptionCodeChange: (code: string) => void
  onRedeem: () => void
  redeeming: boolean
  topupLink?: string
  loading?: boolean
  priceRatio?: number
  usdExchangeRate?: number
  onOpenBilling?: () => void
  creemProducts?: CreemProduct[]
  enableCreemTopup?: boolean
  onCreemProductSelect?: (product: CreemProduct) => void
  enableWaffoTopup?: boolean
  waffoPayMethods?: WaffoPayMethod[]
  waffoMinTopup?: number
  onWaffoMethodSelect?: (method: WaffoPayMethod, index: number) => void
  enableWaffoPancakeTopup?: boolean
  blueWhaleQrCode: string | null
  blueWhaleQrWayCode: string | null
  blueWhaleOrderInfo: BlueWhalePayOrderInfo | null
  onCloseBlueWhaleQrCode: () => void
}

export function RechargeFormCard({
  topupInfo,
  presetAmounts,
  selectedPreset,
  onSelectPreset,
  topupAmount,
  onTopupAmountChange,
  paymentAmount,
  calculating,
  onPaymentMethodSelect,
  paymentLoading,
  redemptionCode,
  onRedemptionCodeChange,
  onRedeem,
  redeeming,
  topupLink,
  loading,
  priceRatio = 1,
  usdExchangeRate = 1,
  onOpenBilling,
  creemProducts,
  enableCreemTopup,
  onCreemProductSelect,
  enableWaffoTopup,
  waffoPayMethods,
  waffoMinTopup,
  onWaffoMethodSelect,
  enableWaffoPancakeTopup,
  blueWhaleQrCode,
  blueWhaleQrWayCode,
  blueWhaleOrderInfo,
  onCloseBlueWhaleQrCode,
}: RechargeFormCardProps) {
  const { t } = useTranslation()
  const [localAmount, setLocalAmount] = useState(topupAmount.toString())
  const [countdown, setCountdown] = useState('')
  useEffect(() => {
    if (!blueWhaleOrderInfo) {
      setCountdown('')
      return
    }
    const expire = blueWhaleOrderInfo.createdAt + 2 * 60 * 60 * 1000 // pay 订单 2h 超时
    const update = () => {
      const remain = Math.max(0, expire - Date.now())
      const m = Math.floor(remain / 60000)
      const s = Math.floor((remain % 60000) / 1000)
      setCountdown(`${m}:${s.toString().padStart(2, '0')}`)
    }
    update()
    const timer = setInterval(update, 1000)
    return () => clearInterval(timer)
  }, [blueWhaleOrderInfo])

  useEffect(() => {
    setLocalAmount(topupAmount.toString())
  }, [topupAmount])

  const handleAmountChange = (value: string) => {
    setLocalAmount(value)
    const numValue = Number.parseInt(value) || 0
    if (numValue >= 0) {
      onTopupAmountChange(numValue)
    }
  }

  const enableBlueWhalePay = !!topupInfo?.enable_bluewhalepay_topup
  const hasConfigurableTopup =
    topupInfo?.enable_online_topup ||
    topupInfo?.enable_stripe_topup ||
    enableWaffoTopup ||
    enableWaffoPancakeTopup ||
    enableBlueWhalePay
  const hasAnyTopup = hasConfigurableTopup || enableCreemTopup
  const hasStandardPaymentMethods =
    Array.isArray(topupInfo?.pay_methods) && topupInfo.pay_methods.length > 0
  const hasWaffoPaymentMethods =
    Array.isArray(waffoPayMethods) && waffoPayMethods.length > 0
  const minTopup = getMinTopupAmount(topupInfo)
  // 自定义金额输入框暂时隐藏，等以后优化；恢复显示把 enableCustomAmount 改为 true
  const enableCustomAmount = false
  const redemptionEnabled = topupInfo?.enable_redemption !== false

  if (loading) {
    return (
      <Card data-card-hover='false' className='gap-0 overflow-hidden py-0'>
        <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
          <Skeleton className='h-6 w-32' />
          <Skeleton className='mt-2 h-4 w-48' />
        </CardHeader>
        <CardContent className='space-y-4 p-3 sm:space-y-6 sm:p-5'>
          <div className='space-y-4 sm:space-y-6'>
            {/* Preset Amounts Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-16' />
              <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
                {Array.from({ length: 8 }, (_, index) => `preset-${index}`).map(
                  (key) => (
                    <Skeleton key={key} className='h-[72px] rounded-lg' />
                  )
                )}
              </div>
            </div>

            {/* Custom Amount Input Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-28' />
              <Skeleton className='h-[42px] w-full' />
            </div>

            {/* Payment Methods Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-32' />
              <div className='flex flex-wrap gap-3'>
                {['primary', 'secondary', 'tertiary'].map((key) => (
                  <Skeleton key={key} className='h-10 w-24 rounded-lg' />
                ))}
              </div>
            </div>
          </div>

          {/* Redemption Code Section Skeleton */}
          <div className='space-y-3 border-t pt-8'>
            <Skeleton className='h-3 w-24' />
            <div className='flex gap-2'>
              <Skeleton className='h-10 flex-1' />
              <Skeleton className='h-10 w-20' />
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <TitledCard
      title={t('Add Funds')}
      description={t('Choose an amount and payment method')}
      icon={<WalletCards className='h-4 w-4' />}
      iconTone='success'
      disableHoverEffect
      action={
        onOpenBilling ? (
          <Button
            variant='outline'
            size='sm'
            onClick={onOpenBilling}
            className='w-full gap-2 sm:w-auto'
          >
            <Receipt className='h-4 w-4' />
            {t('Order History')}
          </Button>
        ) : null
      }
      contentClassName='space-y-4 sm:space-y-6'
    >
      {/* Online Topup Section */}
      {hasAnyTopup ? (
        <div className='space-y-4 sm:space-y-6'>
          {hasConfigurableTopup && (
            <>
              {presetAmounts.length > 0 && (
                <div className='space-y-2.5 sm:space-y-3'>
                  <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                    {t('Amount')}
                  </Label>
                  <div className='grid grid-cols-2 gap-1.5 sm:gap-3 md:grid-cols-4'>
                    {presetAmounts.map((preset) => {
                      const discount =
                        preset.discount ||
                        topupInfo?.discount?.[preset.value] ||
                        1.0
                      const {
                        displayValue,
                        actualPrice,
                        savedAmount,
                        hasDiscount,
                      } = calculatePresetPricing(
                        preset.value,
                        priceRatio,
                        discount,
                        usdExchangeRate
                      )
                      return (
                        <Button
                          key={preset.value}
                          variant='outline'
                          className={cn(
                            'flex min-h-16 flex-col items-start rounded-lg px-3 py-2.5 text-left whitespace-normal sm:min-h-[72px] sm:p-4',
                            selectedPreset === preset.value
                              ? 'border-foreground bg-foreground/5 dark:border-foreground dark:bg-foreground/10'
                              : 'border-muted'
                          )}
                          onClick={() => onSelectPreset(preset)}
                        >
                          <div className='flex w-full items-center justify-between'>
                            <div className='text-base font-semibold sm:text-lg'>
                              {formatNumber(displayValue)}
                            </div>
                            {hasDiscount && (
                              <div className='text-xs font-medium text-green-600'>
                                {getDiscountLabel(discount)}
                              </div>
                            )}
                          </div>
                          <div className='text-muted-foreground mt-1.5 w-full text-xs sm:mt-2'>
                            Pay {formatCurrency(actualPrice)}
                            {hasDiscount && savedAmount > 0 && (
                              <span className='text-green-600'>
                                {' '}
                                • Save {formatCurrency(savedAmount)}
                              </span>
                            )}
                          </div>
                        </Button>
                      )
                    })}
                  </div>
                </div>
              )}

              {enableCustomAmount && (
                <div className='space-y-2.5 sm:space-y-3'>
                  <Label
                    htmlFor='topup-amount'
                    className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
                  >
                    {t('Custom Amount')}
                  </Label>
                  <div className='grid grid-cols-[minmax(0,1fr)_minmax(110px,0.55fr)] gap-2 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
                    <Input
                      id='topup-amount'
                      type='number'
                      value={localAmount}
                      onChange={(e) => handleAmountChange(e.target.value)}
                      min={minTopup}
                      placeholder={`Minimum ${minTopup}`}
                      className='h-9 text-base sm:h-10 sm:text-lg'
                    />
                    <div className='bg-muted/30 flex min-h-9 items-center justify-between gap-2 rounded-md border px-3 lg:min-w-52'>
                      <span className='text-muted-foreground truncate text-xs'>
                        {t('Amount to pay:')}
                      </span>
                      {calculating ? (
                        <Skeleton className='h-5 w-16' />
                      ) : (
                        <span className='text-sm font-semibold'>
                          {formatCurrency(paymentAmount)}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              )}

              <div className='space-y-2.5 sm:space-y-3'>
                <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                  {t('Payment Method')}
                </Label>
                {hasStandardPaymentMethods ? (
                  <div className='grid grid-cols-2 gap-1.5 sm:gap-3 lg:grid-cols-3'>
                    {topupInfo?.pay_methods?.map((method) => {
                      const minTopup = method.min_topup || 0
                      const disabled = minTopup > topupAmount
                      const disabledReason = disabled
                        ? t('Minimum topup amount: {{amount}}', {
                            amount: minTopup,
                          })
                        : undefined
                      const disabledLabel = disabled
                        ? `${t('Minimum:')} ${minTopup}`
                        : undefined

                      const button = (
                        <Button
                          key={method.type}
                          variant='outline'
                          onClick={() => onPaymentMethodSelect(method)}
                          disabled={disabled || !!paymentLoading}
                          title={disabledReason}
                          aria-label={
                            disabledReason
                              ? `${method.name}. ${disabledReason}`
                              : method.name
                          }
                          className='min-h-14 min-w-0 justify-start gap-2 rounded-lg px-3 py-2 text-left'
                        >
                          {paymentLoading === method.type ? (
                            <Loader2 className='h-4 w-4 animate-spin' />
                          ) : (
                            getPaymentIcon(
                              method.type,
                              'h-4 w-4',
                              method.icon,
                              method.name
                            )
                          )}
                          <span className='flex min-w-0 flex-col items-start gap-0.5'>
                            <span className='max-w-full truncate'>
                              {method.name}
                            </span>
                            {disabledLabel && (
                              <span className='text-muted-foreground max-w-full truncate text-[11px] leading-4 font-normal'>
                                {disabledLabel}
                              </span>
                            )}
                          </span>
                        </Button>
                      )

                      return disabled ? (
                        <TooltipProvider key={method.type}>
                          <Tooltip>
                            <TooltipTrigger render={button} />
                            <TooltipContent>{disabledReason}</TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                      ) : (
                        button
                      )
                    })}
                  </div>
                ) : null}
                {!hasAnyTopup && (
                  <Alert>
                    <AlertDescription>
                      {t(
                        'No payment methods available. Please contact administrator.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
              </div>

              {enableWaffoTopup &&
                hasWaffoPaymentMethods &&
                onWaffoMethodSelect && (
                  <div className='space-y-2.5 sm:space-y-3'>
                    <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                      {t('Waffo Payment')}
                    </Label>
                    <div className='grid grid-cols-2 gap-1.5 sm:gap-3 lg:grid-cols-3'>
                      {waffoPayMethods?.map((method, index) => {
                        const loadingKey = `waffo-${index}`
                        const methodKey = `${method.payMethodType ?? 'unknown'}-${method.payMethodName ?? method.name}`
                        const waffoMin = waffoMinTopup || 0
                        const belowMin = waffoMin > topupAmount
                        const disabledReason = belowMin
                          ? t('Minimum topup amount: {{amount}}', {
                              amount: waffoMin,
                            })
                          : undefined
                        const disabledLabel = belowMin
                          ? `${t('Minimum:')} ${waffoMin}`
                          : undefined

                        let methodIcon = getPaymentIcon('waffo')
                        if (paymentLoading === loadingKey) {
                          methodIcon = (
                            <Loader2 className='h-4 w-4 animate-spin' />
                          )
                        } else if (method.icon) {
                          methodIcon = (
                            <img
                              src={method.icon}
                              alt={method.name}
                              className='h-4 w-4 object-contain'
                            />
                          )
                        }

                        const button = (
                          <Button
                            key={methodKey}
                            variant='outline'
                            onClick={() => onWaffoMethodSelect(method, index)}
                            disabled={belowMin || !!paymentLoading}
                            title={disabledReason}
                            aria-label={
                              disabledReason
                                ? `${method.name}. ${disabledReason}`
                                : method.name
                            }
                            className='min-h-14 min-w-0 justify-start gap-2 rounded-lg px-3 py-2 text-left'
                          >
                            {methodIcon}
                            <span className='flex min-w-0 flex-col items-start gap-0.5'>
                              <span className='max-w-full truncate'>
                                {method.name}
                              </span>
                              {disabledLabel && (
                                <span className='text-muted-foreground max-w-full truncate text-[11px] leading-4 font-normal'>
                                  {disabledLabel}
                                </span>
                              )}
                            </span>
                          </Button>
                        )

                        return belowMin ? (
                          <TooltipProvider key={methodKey}>
                            <Tooltip>
                              <TooltipTrigger render={button} />
                              <TooltipContent>{disabledReason}</TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        ) : (
                          button
                        )
                      })}
                    </div>
                  </div>
                )}

              {/* 扫码支付弹窗：bluewhalepay_alipay / bluewhalepay_wechat 选定后，
                  由 Wallet handlePaymentConfirm 调 hook 下单，此处仅展示二维码。 */}
              <Dialog
                open={!!blueWhaleQrCode}
                onOpenChange={(open) => !open && onCloseBlueWhaleQrCode()}
              >
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle className='flex items-center justify-center gap-2'>
                      {blueWhaleQrWayCode === 'ALI_QR' ? (
                        <SiAlipay
                          className='h-5 w-5'
                          style={{ color: '#1677FF' }}
                        />
                      ) : blueWhaleQrWayCode === 'WX_JSAPI' ? (
                        <SiWechat
                          className='h-5 w-5'
                          style={{ color: '#07C160' }}
                        />
                      ) : null}
                      {blueWhaleQrWayCode === 'ALI_QR'
                        ? '支付宝支付'
                        : blueWhaleQrWayCode === 'WX_JSAPI'
                          ? '微信支付'
                          : '扫码支付'}
                    </DialogTitle>
                  </DialogHeader>
                  <div className='flex flex-col items-center gap-3 py-4'>
                    <p className='text-2xl font-bold'>
                      {formatCurrency(paymentAmount)}
                    </p>
                    {blueWhaleQrCode && (
                      <div className='rounded-lg border p-3'>
                        <QRCodeSVG value={blueWhaleQrCode} size={220} />
                      </div>
                    )}
                    <p className='text-muted-foreground text-sm'>
                      请使用
                      {blueWhaleQrWayCode === 'ALI_QR' ? '支付宝' : '微信'}扫描二维码完成支付
                    </p>
                    {blueWhaleOrderInfo?.orderId && (
                      <p className='text-muted-foreground text-xs'>
                        订单号：{blueWhaleOrderInfo.orderId}
                      </p>
                    )}
                    {countdown && (
                      <p className='text-muted-foreground text-xs'>
                        剩余支付时间：{countdown}
                      </p>
                    )}
                    <p className='text-muted-foreground text-xs'>
                      支付完成后额度将自动到账，请勿关闭此窗口
                    </p>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => onCloseBlueWhaleQrCode()}
                    >
                      我已支付完成
                    </Button>
                  </div>
                </DialogContent>
              </Dialog>
            </>
          )}
        </div>
      ) : (
        <Alert>
          <AlertDescription>
            {t(
              'Online topup is not enabled. Please use redemption code or contact administrator.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* Creem Products Section */}
      {enableCreemTopup &&
        Array.isArray(creemProducts) &&
        creemProducts.length > 0 &&
        onCreemProductSelect && (
          <div className='space-y-2.5 border-t pt-4 sm:space-y-3 sm:pt-6'>
            <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Creem Payment')}
            </Label>
            <CreemProductsSection
              products={creemProducts}
              onProductSelect={onCreemProductSelect}
            />
          </div>
        )}

      {/* Redemption Code Section */}
      {redemptionEnabled ? (
        <div className='space-y-2.5 border-t pt-4 sm:space-y-3 sm:pt-6'>
          <div className='flex items-center gap-2'>
            <IconBadge tone='warning' size='xs'>
              <Gift />
            </IconBadge>
            <Label
              htmlFor='redemption-code'
              className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
            >
              {t('Have a Code?')}
            </Label>
          </div>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
            <Input
              id='redemption-code'
              value={redemptionCode}
              onChange={(e) => onRedemptionCodeChange(e.target.value)}
              placeholder={t('Enter your redemption code')}
              className='h-9 min-w-0'
            />
            <Button
              onClick={onRedeem}
              disabled={redeeming}
              variant='outline'
              className='h-9 px-4'
            >
              {redeeming && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              {t('Redeem')}
            </Button>
          </div>
          {topupLink && (
            <p className='text-muted-foreground text-xs'>
              {t('Need a redemption code?')}{' '}
              <a
                href={topupLink}
                target='_blank'
                rel='noopener noreferrer'
                className='inline-flex items-center gap-1 underline-offset-4 hover:underline'
              >
                {t('Get one here')}
                <ExternalLink className='h-3 w-3' />
              </a>
            </p>
          )}
        </div>
      ) : (
        <Alert className='border-t'>
          <AlertDescription>
            {t(
              'Redemption codes are disabled until the administrator confirms compliance terms.'
            )}
          </AlertDescription>
        </Alert>
      )}
    </TitledCard>
  )
}
