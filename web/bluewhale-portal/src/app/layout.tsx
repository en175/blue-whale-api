import type { Metadata } from 'next'

import './global.css'

export const metadata: Metadata = {
  title: 'BLUE WHALE API',
  description: 'Seedance 2.0 视频生成 API',
  robots: {
    follow: false,
    index: false
  }
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang='zh-CN'>
      <body>{children}</body>
    </html>
  )
}
