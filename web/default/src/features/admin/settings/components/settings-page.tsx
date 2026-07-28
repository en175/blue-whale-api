import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useForm } from 'react-hook-form'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const generalSchema = z.object({
  site_name: z.string().min(1, 'Required'),
  contact_email: z.string().email('Invalid email'),
})

type GeneralValues = z.infer<typeof generalSchema>

function mockSave(delay = 400) {
  return new Promise<void>((r) => setTimeout(r, delay))
}

function GeneralTab() {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)

  const form = useForm<z.infer<typeof generalSchema>>({
    resolver: zodResolver(generalSchema),
    defaultValues: { site_name: 'Blue Whale AI', contact_email: 'admin@bluewhale.ai' },
  })

  async function handleSubmit(_values: GeneralValues) {
    setSaving(true)
    await mockSave()
    setSaving(false)
    toast.success(t('Settings saved'))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('General Settings')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="max-w-md space-y-4">
            <FormField
              control={form.control}
              name="site_name"
              render={(fp) => (
                <FormItem>
                  <FormLabel>{t('Site Name')}</FormLabel>
                  <FormControl>
                    <Input {...fp.field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="contact_email"
              render={(fp) => (
                <FormItem>
                  <FormLabel>{t('Contact Email')}</FormLabel>
                  <FormControl>
                    <Input {...fp.field} type="email" />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button type="submit" disabled={saving}>
              {saving ? t('Saving...') : t('Save')}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}

function RateLimitTab() {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [limit, setLimit] = useState('60')
  const [window, setWindow] = useState('1')

  async function handleSave() {
    setSaving(true)
    await mockSave()
    setSaving(false)
    toast.success(t('Settings saved'))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Rate Limit')}</CardTitle>
      </CardHeader>
      <CardContent className="max-w-md space-y-4">
        <div>
          <label className="text-sm font-medium">{t('Max Requests')}</label>
          <Input
            type="number"
            value={limit}
            onChange={(e) => setLimit(e.target.value)}
            className="mt-1.5"
          />
          <p className="text-muted-foreground mt-1 text-xs">{t('Max requests per window')}</p>
        </div>
        <div>
          <label className="text-sm font-medium">{t('Window (minutes)')}</label>
          <Input
            type="number"
            value={window}
            onChange={(e) => setWindow(e.target.value)}
            className="mt-1.5"
          />
        </div>
        <Button onClick={handleSave} disabled={saving}>
          {saving ? t('Saving...') : t('Save')}
        </Button>
      </CardContent>
    </Card>
  )
}

function PaymentTab() {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [methods, setMethods] = useState({
    wechat: true,
    alipay: true,
    bluewhale: true,
  })

  async function handleSave() {
    setSaving(true)
    await mockSave()
    setSaving(false)
    toast.success(t('Settings saved'))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Payment Methods')}</CardTitle>
      </CardHeader>
      <CardContent className="max-w-md space-y-4">
        {[
          { key: 'wechat', label: 'WeChat Pay' },
          { key: 'alipay', label: 'Alipay' },
          { key: 'bluewhale', label: 'Blue Whale Pay' },
        ].map((item) => (
          <div key={item.key} className="flex items-center justify-between">
            <span className="text-sm">{item.label}</span>
            <Switch
              checked={methods[item.key as keyof typeof methods]}
              onCheckedChange={(v) =>
                setMethods((prev) => ({ ...prev, [item.key]: v }))
              }
            />
          </div>
        ))}
        <Button onClick={handleSave} disabled={saving}>
          {saving ? t('Saving...') : t('Save')}
        </Button>
      </CardContent>
    </Card>
  )
}

export function SettingsPage() {
  const { t } = useTranslation()

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">{t('Settings')}</h1>
      <Tabs defaultValue="general">
        <TabsList>
          <TabsTrigger value="general">{t('General')}</TabsTrigger>
          <TabsTrigger value="rate-limit">{t('Rate Limit')}</TabsTrigger>
          <TabsTrigger value="payment">{t('Payment Methods')}</TabsTrigger>
        </TabsList>
        <TabsContent value="general" className="mt-4">
          <GeneralTab />
        </TabsContent>
        <TabsContent value="rate-limit" className="mt-4">
          <RateLimitTab />
        </TabsContent>
        <TabsContent value="payment" className="mt-4">
          <PaymentTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
