import { publicVideoScope } from './public-video-scope'

const [createTask, getTask] = publicVideoScope.operations

export type DocumentationPage = {
  slug: string
  label: string
  eyebrow: string
  title: string
  summary: string
  sections: Array<{
    heading: string
    body: string
  }>
}

export const documentationPages: DocumentationPage[] = [
  {
    slug: 'quickstart',
    label: '快速开始',
    eyebrow: 'START HERE',
    title: '从第一个任务开始',
    summary: 'V1 聚焦提交任务与查询任务两个公开操作。',
    sections: [
      {
        heading: '1. 先确认公开范围',
        body:
          '当前门户只展示 ' +
          publicVideoScope.model +
          ' 与两条任务操作。鉴权与字段细节以正式发布的 V1 契约为准。'
      },
      {
        heading: '2. 提交视频生成任务',
        body:
          '使用 ' +
          createTask.method +
          ' ' +
          createTask.path +
          ' 提交任务。具体请求内容会在公开契约确认后提供。'
      },
      {
        heading: '3. 查询任务进展',
        body:
          '使用 ' +
          getTask.method +
          ' ' +
          getTask.path +
          ' 查询任务。状态与响应细节只以 V1 契约为准。'
      }
    ]
  },
  {
    slug: 'authentication',
    label: '认证说明',
    eyebrow: 'AUTHENTICATION',
    title: '认证信息以 V1 契约为准',
    summary: '门户不会接收、储存或代理你的密钥。',
    sections: [
      {
        heading: '服务端保管',
        body: '请勿在浏览器、客户端包或公开代码仓库中放入真实密钥。'
      },
      {
        heading: '正式入口',
        body: '认证方式与控制台入口将在正式环境配置和 V1 契约确认后发布。'
      }
    ]
  },
  {
    slug: 'seedance-2',
    label: 'Seedance 2.0',
    eyebrow: 'MODEL',
    title: '唯一公开的视频生成模型',
    summary: 'V1 仅展示 seedance-2.0。',
    sections: [
      {
        heading: '公开范围',
        body: '模型能力、参数取值与默认值只会在经过批准的 V1 契约中发布。'
      },
      {
        heading: '任务流程',
        body: '从提交任务开始，再通过查询操作跟进任务进展。'
      }
    ]
  },
  {
    slug: 'create-task',
    label: '提交任务',
    eyebrow: 'POST',
    title: createTask.path,
    summary: '提交 seedance-2.0 视频生成任务。',
    sections: [
      {
        heading: '模型',
        body: 'V1 的唯一公开模型为 ' + publicVideoScope.model + '。'
      },
      {
        heading: '请求与响应',
        body: '完整 JSON 字段会随批准的 V1 契约一同发布；门户不会把内部传输字段视为公开接口。'
      }
    ]
  },
  {
    slug: 'get-task',
    label: '查询任务',
    eyebrow: 'GET',
    title: getTask.path,
    summary: '使用任务标识查询生成任务。',
    sections: [
      {
        heading: '状态与结果',
        body: '状态枚举和结果字段以批准契约为准。请勿依赖内部任务对象或历史文档的响应结构。'
      },
      {
        heading: '保守接入',
        body: '遇到异常时，请以正式发布的错误处理说明为准。'
      }
    ]
  },
  {
    slug: 'errors',
    label: '错误码与限流',
    eyebrow: 'RELIABILITY',
    title: '以 V1 契约为准的错误处理',
    summary: 'HTTP 状态和业务错误码将在公开契约冻结后列出。',
    sections: [
      {
        heading: '当前原则',
        body: '不要根据旧的全量网关文档推断错误码、限流口径或处理策略。'
      }
    ]
  },
  {
    slug: 'pricing',
    label: '费用说明',
    eyebrow: 'BILLING',
    title: '费用以官方公告为准',
    summary: '本门户不展示任何交易与配额营销信息。',
    sections: [
      {
        heading: '公开说明',
        body: '费用以官方公告为准。'
      }
    ]
  },
  {
    slug: 'faq',
    label: 'FAQ',
    eyebrow: 'FAQ',
    title: '面向接入的常见问题',
    summary: '仅回答当前 V1 范围内的问题。',
    sections: [
      {
        heading: '为什么只有一个模型？',
        body: 'V1 只对外提供 seedance-2.0，避免把未批准能力带入公开面。'
      },
      {
        heading: '为什么页面没有完整字段表？',
        body: '字段表会在公开契约冻结后发布，确保文档与实际接口保持一致。'
      }
    ]
  }
]
