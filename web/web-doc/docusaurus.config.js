import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Blue Whale API',
  tagline: 'AI 模型接口文档',
  // 生产域名占位，上线前替换为真实域名（影响 sitemap / canonical URL）
  url: 'https://blue-whale.example.com',
  baseUrl: '/',
  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          // 文档站无 blog，文档作为站点根路由
          routeBasePath: '/',
          sidebarPath: './sidebars.js',
          // 用 openapi theme 的 ApiItem 作为 doc item layout（包 Redux Provider，渲染接口交互组件）
          docItemComponent: '@theme/ApiItem',
        },
        blog: false,
      }),
    ],
  ],

  // openapi 渲染主题（提供 @theme/ApiExplorer / ParamsDetails / RequestSchema 等组件）
  themes: ['docusaurus-theme-openapi-docs'],

  plugins: [
    [
      'docusaurus-plugin-openapi-docs',
      {
        id: 'openapi',
        docsPluginId: 'classic',
        config: {
          ai: {
            // 仓库现成的 AI 模型接口 spec（35 接口）
            specPath: 'openapi/relay.json',
            outputDir: 'docs/api',
            sidebarOptions: {
              groupPathsBy: 'tag',
            },
          },
        },
      },
    ],
  ],

  themeConfig: {
    navbar: {
      title: 'Blue Whale API',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'defaultSidebar',
          position: 'left',
          label: '接口文档',
        },
      ],
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
    // 请求示例只保留 curl/java/python/go（覆盖 theme 默认全语言列表）
    languageTabs: [
      { language: 'curl' },
      { language: 'java' },
      { language: 'python' },
      { language: 'go' },
    ],
  },
};

export default config;
