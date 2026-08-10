export const publicVideoScope = {
  model: 'seedance-2.0',
  operations: [
    { method: 'POST', path: '/v1/video/generations' },
    { method: 'GET', path: '/v1/video/generations/{task_id}' }
  ]
} as const

export const publicPortalLanguages = ['zh'] as const

export type PortalLanguage = (typeof publicPortalLanguages)[number]

export function getPublicPortalLanguage(language: string): PortalLanguage | undefined {
  return publicPortalLanguages.find((item) => item === language)
}
