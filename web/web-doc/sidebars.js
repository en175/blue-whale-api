// ⚠️ docs/api/sidebar.ts 由 `npm run gen-api-docs` 生成（gitignored）。
// clone 后必须先跑 gen-api-docs，否则此 import 失败会导致 build/start 崩溃。
import apisidebar from "./docs/api/sidebar";

// newapi sidebar 大类顺序（字母序）
const TOP_ORDER = [
  "音频（Audio）",
  "聊天（Chat）",
  "补全（Completions）",
  "嵌入（Embeddings）",
  "图像（Images）",
  "模型（Models）",
  "审查（Moderations）",
  "实时语音（Realtime）",
  "重排序（Rerank）",
  "未实现（Unimplemented）",
  "视频（Videos）",
];

// 把 slice 的单层 category（label "大类/格式"）拆成两层：大类 > 格式 > 接口
// 无 "/" 的 category（如 补全（Completions））接口直挂大类
function buildTree(items) {
  const cats = {};
  for (const it of items) {
    if (it.type !== "category") continue; // 跳过 info doc
    const i = it.label.indexOf("/");
    const top = i === -1 ? it.label : it.label.slice(0, i);
    const fmt = i === -1 ? null : it.label.slice(i + 1);
    (cats[top] = cats[top] || []).push({ fmt, items: it.items });
  }
  return TOP_ORDER.filter((t) => cats[t]).map((top) => {
    const subs = cats[top];
    const sub = subs
      .map((s) =>
        s.fmt === null
          ? s.items
          : [{ type: "category", label: s.fmt, items: s.items, collapsed: true }]
      )
      .flat();
    return { type: "category", label: top, items: sub, collapsed: false };
  });
}

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  defaultSidebar: [{ type: "doc", id: "intro" }, ...buildTree(apisidebar)],
};

export default sidebars;
