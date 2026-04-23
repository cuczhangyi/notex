<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { request } from "../api/request";
import type { NoteItem } from "../stores/note";
import type { SourceItem } from "../stores/source";

type MarkedLike = {
  parse: (markdown: string) => string;
};

type MermaidLike = {
  initialize: (config: Record<string, unknown>) => void;
  render: (id: string, code: string) => Promise<{ svg: string }>;
};

type MermaidViewportControllerLike = {
  zoomIn: () => void;
  zoomOut: () => void;
  reset: () => void;
};

declare global {
  interface Window {
    marked?: MarkedLike;
    mermaid?: MermaidLike;
  }
}

interface PublicNotebook {
  id: string;
  name: string;
  description?: string;
  created_at?: string;
}

const route = useRoute();
const router = useRouter();

const loading = ref(false);
const error = ref("");
const notebook = ref<PublicNotebook | null>(null);
const sources = ref<SourceItem[]>([]);
const notes = ref<NoteItem[]>([]);
const selectedSourceId = ref("");
const selectedNoteId = ref("");
const activeCenterTab = ref<"notes_list" | "note">("notes_list");
const noteContentRef = ref<HTMLDivElement | null>(null);
let mermaidInitialized = false;
const transformOptions = [
  { type: "summary", label: "摘要" },
  { type: "faq", label: "常见问题" },
  { type: "study_guide", label: "学习指南" },
  { type: "outline", label: "大纲" },
  { type: "podcast", label: "播客" },
  { type: "timeline", label: "时间线" },
  { type: "glossary", label: "术语表" },
  { type: "quiz", label: "测验" },
  { type: "mindmap", label: "思维导图" },
  { type: "infograph", label: "信息图" },
  { type: "ppt", label: "幻灯片" },
  { type: "insight", label: "洞察" },
  { type: "data_table", label: "数据表格" },
  { type: "data_chart", label: "数据图表" },
];
const markdownNoteTypes = new Set([
  "summary",
  "faq",
  "study_guide",
  "outline",
  "blog",
  "timeline",
]);

const token = computed(() => String(route.params.token || ""));
const selectedSource = computed(() =>
  sources.value.find((item) => item.id === selectedSourceId.value) || null,
);
const selectedNote = computed(() => notes.value.find((item) => item.id === selectedNoteId.value) || null);
const shouldRenderMarkdownNote = computed(() => {
  if (!selectedNote.value?.content) {
    return false;
  }
  return markdownNoteTypes.has(selectedNote.value.type);
});
const shouldRenderMindmapNote = computed(() => {
  if (!selectedNote.value?.content) {
    return false;
  }
  return selectedNote.value.type === "mindmap";
});
const shouldRenderRichNote = computed(() => {
  return shouldRenderMarkdownNote.value || shouldRenderMindmapNote.value;
});
const selectedNoteRenderedHtml = computed(() => {
  if (!selectedNote.value?.content) {
    return "";
  }
  return renderMarkdownContent(selectedNote.value.type, selectedNote.value.content);
});
const sourceFileUrl = computed(() =>
  selectedSource.value?.file_name ? `/api/files/${encodeURIComponent(selectedSource.value.file_name)}` : "",
);
const sourcePreviewType = computed(() => {
  const source = selectedSource.value;
  if (!source) {
    return "none";
  }
  const ext = getExtension(source.file_name || source.name || "");
  if (["png", "jpg", "jpeg", "gif", "webp", "svg"].includes(ext)) {
    return "image";
  }
  if (ext === "pdf") {
    return "pdf";
  }
  if (["md", "markdown", "txt"].includes(ext) || source.type === "text") {
    return "markdown";
  }
  return "unknown";
});

/**
 * 获取文件扩展名
 */
function getExtension(name: string): string {
  const idx = name.lastIndexOf(".");
  if (idx < 0) {
    return "";
  }
  return name.slice(idx + 1).toLowerCase();
}

/**
 * 格式化文件大小
 */
function formatSize(bytes?: number): string {
  if (!bytes || bytes <= 0) {
    return "文本来源";
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * 提取笔记预览文本
 */
function toPlainText(markdown: string): string {
  return markdown
    .replace(/^#+\s+/gm, "")
    .replace(/\*\*/g, "")
    .replace(/\*/g, "")
    .replace(/`/g, "")
    .replace(/\n+/g, " ")
    .trim();
}

/**
 * 转义 HTML 文本
 */
function escapeHTML(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * 按旧版逻辑规整思维导图内容，兼容原始 Mermaid 文本和代码块两种格式
 */
function normalizeMindmapMarkdown(content: string): string {
  const trimmed = content.trim();
  if (!trimmed) {
    return content;
  }

  const fencedMermaidMatch = trimmed.match(/^```mermaid\s*([\s\S]*?)```$/i);
  if (fencedMermaidMatch) {
    return `\`\`\`mermaid\n${fencedMermaidMatch[1].trim()}\n\`\`\``;
  }

  const markdownFenceMatch = trimmed.match(/^```[\w-]*\s*[\s\S]*```$/);
  if (markdownFenceMatch) {
    return content;
  }

  const normalizedSource = trimmed.replace(/^mermaid\s*\r?\n/i, "").trim();
  if (/^(mindmap|graph|flowchart)\b/i.test(normalizedSource)) {
    return `\`\`\`mermaid\n${normalizedSource}\n\`\`\``;
  }

  return content;
}

/**
 * 按旧版逻辑将笔记文本转换为 HTML
 */
function renderMarkdownContent(noteType: string, content: string): string {
  const markedLib = window.marked;
  const normalizedContent = noteType === "mindmap" ? normalizeMindmapMarkdown(content) : content;
  if (!markedLib) {
    return `<pre>${escapeHTML(normalizedContent)}</pre>`;
  }
  return markedLib.parse(normalizedContent);
}

/**
 * 按旧版规则清洗 Mermaid 思维导图源码
 */
function sanitizeMermaidCode(code: string): string {
  let sanitized = code.trim();

  if (sanitized.startsWith("graph")) {
    sanitized = sanitized.replace(/(\s+)-->(\s+)([^"\s][^-\n>]*\([^)]*\)[^-\n>]*)/g, '$1-->$2"$3"');
    sanitized = sanitized.replace(/([^"\s][^-\n>]*\([^)]*\)[^-\n>]*)\s+-->/g, '"$1" -->');
  }

  if (!sanitized.startsWith("mindmap")) {
    return sanitized;
  }

  const lines = sanitized.split("\n");
  const processedLines: string[] = [];

  lines.forEach((line) => {
    const trimmed = line.trim();
    if (!trimmed || trimmed === "mindmap") {
      processedLines.push(line);
      return;
    }

    if (trimmed.startsWith("root")) {
      const rootMatch = trimmed.match(/^root\s*(?:\(?\(?["']?(.+?)["']?\)?\)?\))?$/);
      if (rootMatch?.[1]) {
        const rootContent = rootMatch[1].replace(/&quot;/g, "").replace(/["']/g, "").trim();
        const indent = line.match(/^(\s*)/)?.[1] || "";
        processedLines.push(`${indent}root((${rootContent}))`);
        return;
      }
      processedLines.push(line.replace(/root\s*(.+)/, "root(($1))"));
      return;
    }

    const hasSpecialChars = /[\(\)\[\]\{\}"':;,\s]{2,}/.test(trimmed);
    const alreadyQuoted =
      /^["'].*["']$/.test(trimmed) || /^\(.*\)$/.test(trimmed) || /^\[.*\]$/.test(trimmed);

    if (hasSpecialChars && !alreadyQuoted && trimmed.length > 0) {
      const indent = line.match(/^(\s*)/)?.[1] || "";
      processedLines.push(`${indent}"${trimmed.replace(/"/g, '\\"')}"`);
      return;
    }

    processedLines.push(line);
  });

  return processedLines.join("\n");
}

/**
 * 初始化 Mermaid 配置，保持和旧版思维导图风格一致
 */
function ensureMermaidInitialized() {
  if (mermaidInitialized || !window.mermaid) {
    return;
  }
  window.mermaid.initialize({
    startOnLoad: false,
    theme: "base",
    securityLevel: "loose",
    fontFamily: "var(--font-sans)",
    themeVariables: {
      primaryColor: "#f8fafc",
      primaryTextColor: "#0f172a",
      primaryBorderColor: "#818cf8",
      lineColor: "#94a3b8",
      secondaryColor: "#eef2ff",
      tertiaryColor: "#ffffff",
      fontSize: "14px",
      mainBkg: "#f8fafc",
      nodeBorder: "#818cf8",
      clusterBkg: "#eef2ff",
      nodeTextColor: "#0f172a",
      edgeColor: "#a5b4fc",
    },
    mindmap: {
      useMaxWidth: true,
      padding: 20,
    },
  });
  mermaidInitialized = true;
}

/**
 * 解析 SVG 数值属性，忽略百分比等不稳定尺寸
 */
function parseSvgSize(value: string | null): number {
  if (!value || value.includes("%")) {
    return 0;
  }
  const matched = value.match(/-?\d+(?:\.\d+)?/);
  return matched ? Number(matched[0]) : 0;
}

/**
 * 规范 Mermaid 输出的 SVG 尺寸，避免出现 0 宽高导致空白
 */
function normalizeMermaidSvg(svgElement: SVGSVGElement, renderId: string) {
  svgElement.id = `${renderId}-svg`;

  const viewBox = svgElement.getAttribute("viewBox");
  let width = parseSvgSize(svgElement.getAttribute("width"));
  let height = parseSvgSize(svgElement.getAttribute("height"));

  if (viewBox) {
    const parts = viewBox
      .split(/[\s,]+/)
      .map((item) => Number(item))
      .filter((item) => Number.isFinite(item));
    if (parts.length === 4) {
      width = parts[2] || width;
      height = parts[3] || height;
    }
  }

  if (!viewBox && width > 0 && height > 0) {
    svgElement.setAttribute("viewBox", `0 0 ${width} ${height}`);
  }

  const finalWidth = width > 0 ? width : 960;
  const finalHeight = height > 0 ? height : 560;

  svgElement.removeAttribute("width");
  svgElement.removeAttribute("height");
  svgElement.setAttribute("preserveAspectRatio", "xMinYMin meet");
  svgElement.style.width = `${finalWidth}px`;
  svgElement.style.height = `${finalHeight}px`;
  svgElement.style.maxWidth = "none";
  svgElement.style.minWidth = `${finalWidth}px`;
  svgElement.style.transformOrigin = "top left";
  svgElement.style.transform = "scale(1)";
}

/**
 * 为 Mermaid 思维导图创建缩放与拖拽控制器
 */
function createMermaidViewportController(
  canvas: HTMLDivElement,
  svgElement: SVGSVGElement,
): MermaidViewportControllerLike {
  const minScale = 0.6;
  const maxScale = 2.4;
  const step = 0.16;
  let scale = 1;
  let dragging = false;
  let pointerId = -1;
  let startX = 0;
  let startY = 0;
  let startScrollLeft = 0;
  let startScrollTop = 0;

  const applyScale = () => {
    svgElement.style.transform = `scale(${scale})`;
  };

  const clampScale = (value: number) => {
    return Math.min(maxScale, Math.max(minScale, Number(value.toFixed(2))));
  };

  const stopDragging = () => {
    dragging = false;
    canvas.classList.remove("dragging");
    if (pointerId >= 0 && canvas.hasPointerCapture(pointerId)) {
      canvas.releasePointerCapture(pointerId);
    }
    pointerId = -1;
  };

  canvas.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    dragging = true;
    pointerId = event.pointerId;
    startX = event.clientX;
    startY = event.clientY;
    startScrollLeft = canvas.scrollLeft;
    startScrollTop = canvas.scrollTop;
    canvas.classList.add("dragging");
    canvas.setPointerCapture(pointerId);
  });

  canvas.addEventListener("pointermove", (event) => {
    if (!dragging || event.pointerId !== pointerId) {
      return;
    }
    event.preventDefault();
    canvas.scrollLeft = startScrollLeft - (event.clientX - startX);
    canvas.scrollTop = startScrollTop - (event.clientY - startY);
  });

  canvas.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();

      const previousScale = scale;
      const nextScale = clampScale(
        scale + (event.deltaY < 0 ? step : -step),
      );
      if (nextScale === previousScale) {
        return;
      }

      const rect = canvas.getBoundingClientRect();
      const anchorX = event.clientX - rect.left + canvas.scrollLeft;
      const anchorY = event.clientY - rect.top + canvas.scrollTop;
      const ratio = nextScale / previousScale;

      scale = nextScale;
      applyScale();

      canvas.scrollLeft = anchorX * ratio - (event.clientX - rect.left);
      canvas.scrollTop = anchorY * ratio - (event.clientY - rect.top);
    },
    { passive: false },
  );

  canvas.addEventListener("pointerup", stopDragging);
  canvas.addEventListener("pointercancel", stopDragging);
  canvas.addEventListener("lostpointercapture", stopDragging);
  canvas.addEventListener("dragstart", (event) => event.preventDefault());

  applyScale();

  return {
    zoomIn: () => {
      scale = clampScale(scale + step);
      applyScale();
    },
    zoomOut: () => {
      scale = clampScale(scale - step);
      applyScale();
    },
    reset: () => {
      scale = 1;
      applyScale();
      canvas.scrollTo({ left: 0, top: 0, behavior: "smooth" });
    },
  };
}

/**
 * 为 Mermaid 思维导图创建缩放工具栏
 */
function createMermaidToolbar(controller: MermaidViewportControllerLike) {
  const toolbar = document.createElement("div");
  toolbar.className = "mermaid-diagram-toolbar";

  const actions = [
    {
      label: "-",
      title: "缩小",
      onClick: () => controller.zoomOut(),
    },
    {
      label: "+",
      title: "放大",
      onClick: () => controller.zoomIn(),
    },
    {
      label: "1:1",
      title: "重置",
      onClick: () => controller.reset(),
    },
  ];

  actions.forEach((action) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "mermaid-toolbar-button";
    button.textContent = action.label;
    button.title = action.title;
    button.setAttribute("aria-label", action.title);
    button.addEventListener("click", action.onClick);
    toolbar.appendChild(button);
  });

  const hint = document.createElement("span");
  hint.className = "mermaid-diagram-hint";
  hint.textContent = "拖动画布，滚轮缩放，点击按钮重置";
  toolbar.appendChild(hint);

  return toolbar;
}

/**
 * 包装 Mermaid SVG，并增强为专业脑图交互视图
 */
function createMermaidDiagramElement(svg: string, renderId: string) {
  const diagram = document.createElement("div");
  diagram.className = "mermaid-diagram";

  const canvas = document.createElement("div");
  canvas.className = "mermaid-diagram-canvas";
  canvas.innerHTML = svg;
  diagram.appendChild(canvas);

  const svgElement = canvas.querySelector("svg");
  if (!(svgElement instanceof SVGSVGElement)) {
    return diagram;
  }

  normalizeMermaidSvg(svgElement, renderId);
  canvas.style.touchAction = "none";
  const controller = createMermaidViewportController(canvas, svgElement);
  diagram.prepend(createMermaidToolbar(controller));
  return diagram;
}

/**
 * 将 Markdown 中的 Mermaid 代码块渲染为 SVG 图
 */
async function renderMermaidDiagrams(container: HTMLElement | null) {
  if (!container || !window.mermaid) {
    return;
  }

  ensureMermaidInitialized();
  const mermaidBlocks = container.querySelectorAll("pre code.language-mermaid");
  if (mermaidBlocks.length === 0) {
    return;
  }

  for (const [index, block] of mermaidBlocks.entries()) {
    const pre = block.parentElement;
    if (!pre) {
      continue;
    }

    const rawCode = block.textContent || "";
    const cleanCode = sanitizeMermaidCode(rawCode);
    const renderId = `public-mermaid-diag-${Date.now()}-${index}`;

    try {
      const { svg } = await window.mermaid.render(renderId, cleanCode);
      const diagram = createMermaidDiagramElement(svg, renderId);
      pre.replaceWith(diagram);
    } catch (error) {
      try {
        const { svg } = await window.mermaid.render(`${renderId}-retry`, cleanCode.replace(/\(|\)/g, ""));
        const diagram = createMermaidDiagramElement(svg, `${renderId}-retry`);
        pre.replaceWith(diagram);
      } catch (retryError) {
        const errorNode = document.createElement("div");
        errorNode.style.color = "red";
        errorNode.style.fontSize = "12px";
        errorNode.style.padding = "10px";
        errorNode.textContent =
          `渲染失败: ${retryError instanceof Error ? retryError.message : String(retryError)}`;
        pre.replaceChildren(errorNode);
      }
      console.warn("Mermaid 渲染失败:", error);
    }
  }
}

/**
 * 在公开页笔记详情中渲染 Mermaid 思维导图
 */
async function renderSelectedNoteMermaid() {
  if (activeCenterTab.value !== "note" || !shouldRenderRichNote.value) {
    return;
  }
  await nextTick();
  await renderMermaidDiagrams(noteContentRef.value);
}

/**
 * 返回首页
 */
function backToList() {
  router.push("/");
}

/**
 * 加载公开笔记本数据
 */
async function loadPublicNotebook() {
  if (!token.value) {
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    const [notebookResp, sourceResp, noteResp] = await Promise.all([
      request<PublicNotebook>(`/public/notebooks/${token.value}`, { withAuth: false }),
      request<SourceItem[]>(`/public/notebooks/${token.value}/sources`, { withAuth: false }),
      request<NoteItem[]>(`/public/notebooks/${token.value}/notes`, { withAuth: false }),
    ]);
    notebook.value = notebookResp;
    sources.value = sourceResp;
    notes.value = noteResp;
    selectedSourceId.value = sourceResp[0]?.id || "";
    selectedNoteId.value = noteResp[0]?.id || "";
    activeCenterTab.value = "notes_list";
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

watch(
  () => token.value,
  async () => {
    await loadPublicNotebook();
  },
);

watch(
  () => [selectedNote.value?.id, selectedNote.value?.content, activeCenterTab.value],
  () => {
    void renderSelectedNoteMermaid();
  },
);

onMounted(async () => {
  await loadPublicNotebook();
});
</script>

<template>
  <main class="app-container">
    <div class="workspace-container readonly-mode">
      <nav class="workspace-nav">
        <button class="btn-back" @click="backToList">返回列表</button>
        <div class="current-notebook-info">
          <h2 class="notebook-name-display">{{ notebook?.name || "公开笔记本" }}</h2>
          <div class="public-badge">
            <span>公开</span>
          </div>
        </div>
        <div class="workspace-actions">
          <button class="btn-new-notebook" @click="backToList">返回首页</button>
        </div>
      </nav>

      <section v-if="loading" class="card">
        <p>加载公开笔记本数据中...</p>
      </section>
      <section v-else-if="error" class="card">
        <p class="error">加载失败：{{ error }}</p>
      </section>

      <main v-else class="main-grid public-main-grid">
        <aside class="panel panel-left">
          <div class="panel-header">
            <h2 class="panel-title">来源</h2>
            <span class="panel-count">{{ sources.length }}</span>
          </div>
          <ul class="source-list">
            <li
              v-for="item in sources"
              :key="item.id"
              class="source-item"
              :class="{ active: selectedSourceId === item.id }"
              @click="selectedSourceId = item.id"
            >
              <div class="title">{{ item.name }}</div>
              <div class="meta">
                {{ item.type }} · {{ formatSize(item.file_size) }} · {{ item.status || "completed" }}
              </div>
            </li>
          </ul>

          <div class="resource-preview">
            <h3>资源预览</h3>
            <p v-if="!selectedSource" class="muted">暂无来源</p>
            <div v-else-if="sourcePreviewType === 'image'" class="preview-image-wrap">
              <img class="preview-image" :src="sourceFileUrl" :alt="selectedSource.name" />
            </div>
            <div v-else-if="sourcePreviewType === 'pdf'" class="preview-pdf-wrap">
              <iframe class="preview-pdf" :src="sourceFileUrl" title="public-source-pdf-preview" />
            </div>
            <pre v-else-if="sourcePreviewType === 'markdown'" class="preview-text">
{{ selectedSource.content || "内容处理中..." }}</pre
            >
            <p v-else class="muted">当前类型暂不支持内嵌预览。</p>
          </div>
        </aside>

        <section class="panel panel-center">
          <div class="panel-header">
            <div class="panel-tabs">
              <button
                class="tab-btn"
                :class="{ active: activeCenterTab === 'notes_list' }"
                @click="activeCenterTab = 'notes_list'"
              >
                笔记列表
              </button>
              <button
                class="tab-btn"
                :class="{ active: activeCenterTab === 'note' }"
                :disabled="!selectedNote"
                @click="activeCenterTab = 'note'"
              >
                笔记
              </button>
            </div>
          </div>

          <div v-if="activeCenterTab === 'notes_list'" class="chat-sessions-panel">
            <ul class="note-list">
              <li
                v-for="item in notes"
                :key="item.id"
                class="note-item"
                :class="{ active: selectedNoteId === item.id }"
                @click="
                  selectedNoteId = item.id;
                  activeCenterTab = 'note';
                "
              >
                <div class="title">{{ item.title }}</div>
                <div class="meta">{{ item.type }} · {{ item.source_ids?.length || 0 }} 来源</div>
                <div class="preview">{{ toPlainText(item.content) }}</div>
              </li>
            </ul>
          </div>

          <div v-else class="chat-panel">
            <div v-if="selectedNote" class="note-detail">
              <h3>{{ selectedNote.title }}</h3>
              <div class="note-meta">{{ selectedNote.type }}</div>
              <div
                v-if="shouldRenderRichNote"
                ref="noteContentRef"
                class="markdown-content"
                v-html="selectedNoteRenderedHtml"
              ></div>
              <pre v-else>{{ selectedNote.content }}</pre>
            </div>
            <p v-else class="muted">暂无笔记</p>
          </div>
        </section>

        <aside class="panel panel-right readonly-panel">
          <div class="workspace-section section-transform">
            <div class="panel-header studio-header">
              <h2 class="panel-title">STUDIO</h2>
            </div>
            <div class="transform-grid">
              <button
                v-for="option in transformOptions"
                :key="option.type"
                class="transform-card"
                :data-type="option.type"
                disabled
              >
                <span class="transform-icon">✦</span>
                <span class="transform-name">{{ option.label }}</span>
              </button>
            </div>
            <p class="muted readonly-tip">只读视图，不支持编辑、对话、转换与删除操作。</p>
          </div>
        </aside>
      </main>
    </div>
  </main>
</template>
