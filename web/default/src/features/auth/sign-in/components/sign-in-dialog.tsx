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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { TermsFooter } from '@/features/auth/components/terms-footer'
import { UserAuthForm } from '@/features/auth/sign-in/components/user-auth-form'
import { useStatus } from '@/hooks/use-status'

interface SignInDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SignInDialog({ open, onOpenChange }: SignInDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Sign in')}
      description={
        status?.register_enabled !== false ? (
          <>
            {t("Don't have an account?")}{' '}
            <Link
              to='/sign-up'
              className='hover:text-primary font-medium underline underline-offset-4'
              onClick={() => onOpenChange(false)}
            >
              {t('Sign up')}
            </Link>
          </>
        ) : undefined
      }
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      headerClassName='text-left'
      bodyClassName='space-y-6'
    >
      <UserAuthForm />
      <TermsFooter variant='sign-in' status={status} className='text-center' />
    </Dialog>
  )
}
