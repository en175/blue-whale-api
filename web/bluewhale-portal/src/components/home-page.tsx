import { publicVideoScope, type PortalLanguage } from '../lib/public-video-scope'

const [createTask, getTask] = publicVideoScope.operations

type HomePageProps = {
  language: PortalLanguage
}

export function HomePage({ language }: HomePageProps) {
  const docsHref = '/' + language + '/docs'

  return (
    <main className='site-shell'>
      <header className='site-header'>
        <a className='brand' href={'/' + language} aria-label='BLUE WHALE API 首页'>
          <span className='brand-mark' aria-hidden='true'>
            ≋
          </span>
          <span>BLUE WHALE API</span>
        </a>
        <nav className='site-nav' aria-label='主导航'>
          <a href='#capabilities'>视频生成</a>
          <a href={docsHref}>文档</a>
          <span className='nav-pending'>控制台将在正式环境配置</span>
        </nav>
      </header>

      <section className='hero'>
        <div className='hero-copy'>
          <p className='eyebrow'>VIDEO GENERATION API</p>
          <h1>
            把想象，
            <br />
            推入镜头。
          </h1>
          <p className='hero-summary'>
            面向产品与开发团队的 {publicVideoScope.model} 视频生成入口。让创意从一句描述开始，稳定进入你的工程流程。
          </p>
          <div className='hero-actions'>
            <a className='button button-primary' href={docsHref}>
              查看开发文档
            </a>
            <a className='button button-secondary' href='#quickstart'>
              查看接入流程
            </a>
          </div>
          <div className='hero-signals' aria-label='产品能力'>
            <span>V1 公开范围</span>
            <span>任务流程</span>
            <span>中文优先</span>
          </div>
        </div>
        <div className='hero-stage' aria-label='Seedance 2.0 生成画布'>
          <div className='stage-grid' aria-hidden='true' />
          <div className='stage-orbit orbit-one' />
          <div className='stage-orbit orbit-two' />
          <div className='stage-glow' />
          <div className='stage-card'>
            <div className='stage-card-top'>
              <span className='status-dot' />
              <span>BLUE WHALE CANVAS</span>
              <span>Prompt → Motion</span>
            </div>
            <div className='stage-frame'>
              <span className='frame-light frame-light-one' />
              <span className='frame-light frame-light-two' />
              <span className='frame-wave'>≋</span>
            </div>
            <div className='stage-progress'>
              <span />
            </div>
          </div>
        </div>
      </section>

      <section className='trust-strip' aria-label='公开范围'>
        <span>PUBLIC V1</span>
        <span>{publicVideoScope.model}</span>
        <span>
          {createTask.method} {createTask.path}
        </span>
        <span>
          {getTask.method} {getTask.path}
        </span>
      </section>

      <section className='content-section capabilities' id='capabilities'>
        <div className='section-heading'>
          <p className='eyebrow'>CAPABILITIES</p>
          <h2>让创意进入工程流程。</h2>
        </div>
        <div className='capability-grid'>
          <article className='capability-card'>
            <span className='card-index'>01</span>
            <h3>从描述开始</h3>
            <p>用自然语言描述画面、动作与氛围，让创作从想法进入生成流程。</p>
          </article>
          <article className='capability-card'>
            <span className='card-index'>02</span>
            <h3>围绕任务推进</h3>
            <p>提交生成任务后，再通过查询路径持续跟进任务进展。</p>
          </article>
          <article className='capability-card'>
            <span className='card-index'>03</span>
            <h3>文档跟着实现走</h3>
            <p>先看快速开始，再进入接口说明。每一页只解释当前需要的内容。</p>
          </article>
        </div>
      </section>

      <section className='content-section model-section'>
        <div className='model-panel'>
          <div>
            <p className='eyebrow'>PUBLIC MODEL</p>
            <h2>{publicVideoScope.model}</h2>
            <p>V1 的唯一公开模型。正式发布的 V1 契约会统一说明可接入内容。</p>
          </div>
          <dl className='model-facts'>
            <div>
              <dt>公开模型</dt>
              <dd>{publicVideoScope.model}</dd>
            </div>
            <div>
              <dt>提交方式</dt>
              <dd>提交任务</dd>
            </div>
            <div>
              <dt>查询方式</dt>
              <dd>查询任务</dd>
            </div>
          </dl>
        </div>
      </section>

      <section className='content-section quickstart' id='quickstart'>
        <div className='section-heading'>
          <p className='eyebrow'>QUICKSTART</p>
          <h2>两步接入，开始生成。</h2>
        </div>
        <div className='quickstart-grid'>
          <article className='step-card'>
            <span>01</span>
            <h3>查看接入说明</h3>
            <p>具体鉴权与字段以正式发布的 V1 契约为准。</p>
          </article>
          <article className='step-card'>
            <span>02</span>
            <h3>提交视频生成任务</h3>
            <code>
              {createTask.method} {createTask.path}
            </code>
          </article>
          <article className='step-card'>
            <span>03</span>
            <h3>查询任务进展</h3>
            <code>
              {getTask.method} {getTask.path}
            </code>
          </article>
        </div>
      </section>

      <section className='content-section final-cta'>
        <p className='eyebrow'>BUILD WITH BLUE WHALE</p>
        <h2>先理解公开边界，再开始接入。</h2>
        <a className='button button-primary' href={docsHref}>
          进入文档
        </a>
      </section>

      <footer className='site-footer'>
        <span>BLUE WHALE API</span>
        <span>费用以官方公告为准</span>
        <span>中文优先 · V1</span>
      </footer>
    </main>
  )
}
