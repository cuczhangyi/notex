<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import type { ComponentPublicInstance } from "vue";
import { useRoute, useRouter } from "vue-router";
import { request } from "../api/request";
import { useNotebookStore } from "../stores/notebook";
import { useSourceStore } from "../stores/source";
import { useNoteStore } from "../stores/note";
import { useChatStore } from "../stores/chat";
import type { TransformType } from "../stores/note";
import type { ChatSession } from "../stores/chat";

type PdfViewportLike = {
  width: number;
  height: number;
};

type PdfPageLike = {
  getViewport: (options: { scale: number }) => PdfViewportLike;
  render: (options: {
    canvasContext: CanvasRenderingContext2D;
    viewport: PdfViewportLike;
  }) => { promise: Promise<unknown> };
};

type PdfDocumentLike = {
  numPages: number;
  getPage: (pageNumber: number) => Promise<PdfPageLike>;
  destroy?: () => void;
};

type PdfJsLike = {
  GlobalWorkerOptions: {
    workerSrc: string;
  };
  getDocument: (options: { data: ArrayBuffer }) => {
    promise: Promise<PdfDocumentLike>;
  };
};

type ChartNoteItem = {
  title: string;
  option: Record<string, unknown>;
};

type EChartsInstanceLike = {
  setOption: (option: Record<string, unknown>) => void;
  resize: () => void;
  dispose: () => void;
};

type EChartsLike = {
  init: (
    element: HTMLDivElement,
    theme?: string | null,
    options?: { renderer?: "canvas" | "svg"; useDirtyRect?: boolean },
  ) => EChartsInstanceLike;
};

type MarkedLike = {
  parse: (markdown: string) => string;
};

type MermaidLike = {
  initialize: (config: Record<string, unknown>) => void;
  render: (id: string, code: string) => Promise<{ svg: string }>;
};

declare global {
  interface Window {
    pdfjsLib?: PdfJsLike;
    echarts?: EChartsLike;
    marked?: MarkedLike;
    mermaid?: MermaidLike;
  }
}

const route = useRoute();
const router = useRouter();
const notebookStore = useNotebookStore();
const sourceStore = useSourceStore();
const noteStore = useNoteStore();
const chatStore = useChatStore();
const chatInput = ref("");
const customPrompt = ref("");
const activeCenterTab = ref("chat");
const activeNoteView = ref<"notes_list" | "note_detail">("notes_list");
const openedSourceTabs = ref<string[]>([]);
const activeSourceTabId = ref("");
const promptScenariosCollapsed = ref(true);
const addSourceModalVisible = ref(false);
const addSourceTab = ref<"file" | "text" | "url">("file");
const textSourceName = ref("");
const textSourceContent = ref("");
const urlSourceName = ref("");
const urlSourceValue = ref("");
const leftCollapsed = ref(false);
const rightCollapsed = ref(false);
const textSearchInput = ref("");
const imageZoom = ref(1);
const imageRotate = ref(0);
const textSearchMatchCount = ref(0);
const textSearchCurrentIndex = ref(-1);
const infographStyleDropdownVisible = ref(false);
const selectedInfographStyle = ref<string | null>(null);
const infographStylesLoaded = ref(false);
const infographStyles = ref<Array<{ id: string; name: string; description: string }>>([]);
const pptSlideIndex = ref(0);
const pdfViewportRef = ref<HTMLDivElement | null>(null);
const pdfCanvasRef = ref<HTMLCanvasElement | null>(null);
const pdfLoading = ref(false);
const pdfError = ref("");
const pdfCurrentPage = ref(1);
const pdfTotalPages = ref(0);
const pdfScale = ref(1);
const pdfDocument = shallowRef<PdfDocumentLike | null>(null);
const pdfRequestId = ref(0);
const chartContainerRefs = ref<HTMLDivElement[]>([]);
const chartResizeTimer = ref<ReturnType<typeof setTimeout> | null>(null);
const chartInstances = shallowRef<EChartsInstanceLike[]>([]);
const noteContentRef = ref<HTMLDivElement | null>(null);
const sidebarNoteContentRef = ref<HTMLDivElement | null>(null);
let pollingTimer: ReturnType<typeof setInterval> | null = null;
let documentClickHandler: ((event: MouseEvent) => void) | null = null;
let mermaidInitialized = false;

const notebookId = computed(() => String(route.params.id || ""));
const sources = computed(() => sourceStore.items);
const loading = computed(() => sourceStore.loading);
const error = computed(() => sourceStore.error);
const notes = computed(() => noteStore.items);
const notesLoading = computed(() => noteStore.loading);
const notesError = computed(() => noteStore.error);
const transformingType = computed(() => noteStore.transformingType);
const selectedNote = computed(() => noteStore.selectedItem);
const uploading = computed(() => sourceStore.uploading);
const sessions = computed(() => chatStore.sessions);
const messages = computed(() => chatStore.messages);
const chatError = computed(() => chatStore.error);
const sending = computed(() => chatStore.sending);
const loadingSessions = computed(() => chatStore.loadingSessions);
const activeSessionId = computed(() => chatStore.activeSessionId);
const deletingSessionId = computed(() => chatStore.deletingSessionId);
const clearingSessions = computed(() => chatStore.clearingSessions);
const selectedSourceFileUrl = computed(() =>
  selectedSource.value?.file_name
    ? `/api/files/${encodeURIComponent(selectedSource.value.file_name)}`
    : "",
);
const sourceTabs = computed(() =>
  openedSourceTabs.value
    .map((id) => sources.value.find((item) => item.id === id))
    .filter((item): item is NonNullable<typeof item> => Boolean(item)),
);
const activeSourceTabForCenter = computed(() => {
  return sourceTabs.value.find((item) => item.id === activeCenterTab.value) || null;
});
const selectedSource = computed(() => {
  const activeByTab = sources.value.find((item) => item.id === activeSourceTabId.value);
  if (activeByTab) {
    return activeByTab;
  }
  return sourceStore.selectedItem;
});
const currentNotebook = computed(() =>
  notebookStore.notebooks.find((item) => item.id === notebookId.value) || null,
);
const promptScenarios = [
  { icon: "search", label: "总结核心观点", prompt: "总结这篇文章的核心观点" },
  { icon: "question", label: "列出3个关键问题", prompt: "列出3个关于本文的关键问题" },
  { icon: "lightbulb", label: "概念解释", prompt: "解释文中的重要概念" },
  { icon: "compare", label: "观点对比", prompt: "对比文中的不同观点" },
  { icon: "action", label: "可行建议", prompt: "给出具体可行的建议" },
  { icon: "detail", label: "深入分析", prompt: "深入分析某个具体部分" },
];
const markdownNoteTypes = new Set([
  "summary",
  "faq",
  "study_guide",
  "outline",
  "blog",
  "timeline",
  "glossary",
  "data_table",
]);

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

const previewType = computed(() => {
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

const markdownPreviewText = computed(() => selectedSource.value?.content || "内容处理中...");
const imageTransformStyle = computed(() => ({
  transform: `scale(${imageZoom.value}) rotate(${imageRotate.value}deg)`,
}));
const textSearchCountLabel = computed(() => {
  if (!textSearchInput.value.trim()) {
    return "";
  }
  if (textSearchMatchCount.value === 0) {
    return "无匹配";
  }
  return `${textSearchCurrentIndex.value + 1} / ${textSearchMatchCount.value}`;
});
const highlightedTextPreviewHtml = computed(() => {
  const rawText = markdownPreviewText.value || "";
  const query = textSearchInput.value.trim();
  if (!query) {
    return escapeHTML(rawText);
  }
  const escapedQuery = escapeRegExp(query);
  const regex = new RegExp(escapedQuery, "gi");
  let matchCount = 0;
  const html = escapeHTML(rawText).replace(regex, (value) => {
    const activeClass = matchCount === textSearchCurrentIndex.value ? " active" : "";
    matchCount += 1;
    return `<span class="search-highlight${activeClass}">${escapeHTML(value)}</span>`;
  });
  return html;
});
const pdfPrevDisabled = computed(() => pdfCurrentPage.value <= 1);
const pdfNextDisabled = computed(() => pdfCurrentPage.value >= pdfTotalPages.value);
const pdfZoomLevelLabel = computed(() => `${Math.round(pdfScale.value * 100)}%`);
const selectedInfographStyleName = computed(() => {
  if (!selectedInfographStyle.value) {
    return "";
  }
  return infographStyles.value.find((style) => style.id === selectedInfographStyle.value)?.name || "";
});
const infographStylePickerTitle = computed(() => {
  if (!selectedInfographStyle.value) {
    return "选择风格";
  }
  return `风格: ${selectedInfographStyleName.value || selectedInfographStyle.value}`;
});
const infographStyleOptions = computed(() => {
  return [
    {
      id: "",
      name: "默认风格",
      description: "使用系统默认的手绘纸艺风格",
    },
    ...infographStyles.value,
  ];
});
const parsedChartItems = computed(() => {
  if (selectedNote.value?.type !== "data_chart" || !selectedNote.value.content) {
    return [];
  }
  return parseChartNoteContent(selectedNote.value.content);
});
const shouldRenderDataChart = computed(() => parsedChartItems.value.length > 0);
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
const selectedNotePPTSlides = computed(() => {
  const note = selectedNote.value;
  if (!note || note.type !== "ppt" || !note.metadata || typeof note.metadata !== "object") {
    return [] as string[];
  }
  const slides = (note.metadata as Record<string, unknown>).slides;
  if (!Array.isArray(slides)) {
    return [] as string[];
  }
  return slides.filter((item): item is string => typeof item === "string" && item.trim().length > 0);
});
const shouldRenderPPTSlides = computed(() => selectedNotePPTSlides.value.length > 0);
const currentPPTSlideUrl = computed(() => {
  const slides = selectedNotePPTSlides.value;
  if (slides.length === 0) {
    return "";
  }
  const maxIndex = slides.length - 1;
  const safeIndex = Math.min(Math.max(pptSlideIndex.value, 0), maxIndex);
  return slides[safeIndex] || "";
});
const pptSlideIndicator = computed(() => {
  const total = selectedNotePPTSlides.value.length;
  if (total === 0) {
    return "0 / 0";
  }
  return `${Math.min(pptSlideIndex.value + 1, total)} / ${total}`;
});
const pptPrevDisabled = computed(() => pptSlideIndex.value <= 0);
const pptNextDisabled = computed(() => pptSlideIndex.value >= selectedNotePPTSlides.value.length - 1);
const selectedNoteInfographImageUrl = computed(() => {
  const note = selectedNote.value;
  if (!note || note.type !== "infograph") {
    return "";
  }
  const content = typeof note.content === "string" ? note.content.trim() : "";
  if (content) {
    return "";
  }
  if (!note.metadata || typeof note.metadata !== "object") {
    return "";
  }
  const imageURL = (note.metadata as Record<string, unknown>).image_url;
  if (typeof imageURL !== "string") {
    return "";
  }
  return imageURL.trim();
});
const selectedNoteRenderedHtml = computed(() => {
  if (!selectedNote.value?.content) {
    return "";
  }
  return renderMarkdownContent(selectedNote.value.type, selectedNote.value.content);
});

/**
 * 判断对象是否像 ECharts option 配置
 */
function isEChartsOptionLike(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }

  return [
    "series",
    "xAxis",
    "yAxis",
    "legend",
    "grid",
    "tooltip",
    "dataset",
    "radar",
    "geo",
    "visualMap",
  ].some((key) => key in value);
}

/**
 * 从多种 JSON 结构中提取数据图表项
 */
function normalizeChartItems(payload: unknown): ChartNoteItem[] {
  if (Array.isArray(payload)) {
    return payload
      .map((item) => {
        if (!item || typeof item !== "object") {
          return null;
        }

        const record = item as Record<string, unknown>;
        const option = isEChartsOptionLike(record.option)
          ? record.option
          : isEChartsOptionLike(record)
            ? record
            : null;

        if (!option) {
          return null;
        }

        const optionTitle =
          option.title && typeof option.title === "object"
            ? (option.title as Record<string, unknown>).text
            : "";

        return {
          title:
            typeof record.title === "string"
              ? record.title
              : typeof optionTitle === "string"
                ? optionTitle
                : "",
          option,
        };
      })
      .filter((item): item is ChartNoteItem => item !== null);
  }

  if (!payload || typeof payload !== "object") {
    return [];
  }

  const record = payload as Record<string, unknown>;

  if (Array.isArray(record.charts)) {
    return normalizeChartItems(record.charts);
  }

  if (isEChartsOptionLike(record.option)) {
    const option = record.option;
    const optionTitle =
      option.title && typeof option.title === "object"
        ? (option.title as Record<string, unknown>).text
        : "";

    return [
      {
        title:
          typeof record.title === "string"
            ? record.title
            : typeof optionTitle === "string"
              ? optionTitle
              : "",
        option,
      },
    ];
  }

  if (isEChartsOptionLike(record)) {
    const optionTitle =
      record.title && typeof record.title === "object"
        ? (record.title as Record<string, unknown>).text
        : "";

    return [
      {
        title: typeof optionTitle === "string" ? optionTitle : "",
        option: record,
      },
    ];
  }

  return [];
}

/**
 * 安全解析 JSON 内容，兼容代码块和字符串化 JSON
 */
function parseJsonPayload(content: string): unknown {
  const trimmed = content.trim();
  const normalized = trimmed
    .replace(/^```(?:json|js|javascript)?\s*/i, "")
    .replace(/\s*```$/i, "")
    .trim();

  const candidates = [normalized];
  const jsonMatch = normalized.match(/(\[[\s\S]*\])|(\{[\s\S]*\})/);
  if (jsonMatch?.[0] && jsonMatch[0] !== normalized) {
    candidates.push(jsonMatch[0]);
  }
  // 兼容被转义后的 JSON 文本，例如：[{\"title\":\"...\"}]
  if (normalized.includes('\\"')) {
    candidates.push(
      normalized
        .replace(/\\"/g, '"')
        .replace(/\\n/g, "\n")
        .replace(/\\r/g, "\r")
        .replace(/\\t/g, "\t"),
    );
  }

  for (const candidate of candidates) {
    if (!candidate) {
      continue;
    }

    try {
      let parsed = JSON.parse(candidate) as unknown;
      let depth = 0;
      while (typeof parsed === "string" && depth < 2) {
        parsed = JSON.parse(parsed) as unknown;
        depth += 1;
      }
      return parsed;
    } catch {
      // 继续尝试下一个候选内容
    }
  }

  return null;
}

/**
 * 解析数据图表笔记内容，兼容旧版和数组格式
 */
function parseChartNoteContent(content: string): ChartNoteItem[] {
  try {
    const parsed = parseJsonPayload(content);
    const charts = normalizeChartItems(parsed);
    if (charts.length > 0) {
      return charts;
    }
  } catch (error) {
    console.warn("解析数据图表笔记失败:", error);
  }
  return [];
}

/**
 * 设置图表挂载节点引用
 */
function setChartContainerRef(
  element: Element | ComponentPublicInstance | null,
  index: number,
) {
  if (!(element instanceof HTMLDivElement)) {
    return;
  }
  chartContainerRefs.value[index] = element;
}

/**
 * 销毁当前页面上的图表实例
 */
function disposeChartInstances() {
  chartInstances.value.forEach((instance) => {
    instance.dispose();
  });
  chartInstances.value = [];
  chartContainerRefs.value = [];
  if (chartResizeTimer.value) {
    clearTimeout(chartResizeTimer.value);
    chartResizeTimer.value = null;
  }
}

/**
 * 重绘数据图表笔记中的 ECharts 视图
 */
async function renderSelectedNoteCharts() {
  disposeChartInstances();
  if (activeCenterTab.value !== "note" || !shouldRenderDataChart.value) {
    return;
  }
  const echartsLib = window.echarts;
  if (!echartsLib) {
    console.warn("ECharts 未加载，无法渲染数据图表");
    return;
  }

  await nextTick();

  const instances: EChartsInstanceLike[] = [];
  parsedChartItems.value.forEach((chart, index) => {
    const container = chartContainerRefs.value[index];
    if (!container) {
      return;
    }
    const instance = echartsLib.init(container, undefined, {
      renderer: "canvas",
      useDirtyRect: true,
    });
    instance.setOption(chart.option);
    instances.push(instance);
  });
  chartInstances.value = instances;

  chartResizeTimer.value = setTimeout(() => {
    chartInstances.value.forEach((instance) => instance.resize());
  }, 200);
}

/**
 * 响应窗口尺寸变化，保持图表自适应
 */
function handleChartResize() {
  chartInstances.value.forEach((instance) => instance.resize());
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
 * 加载当前笔记本来源
 */
async function loadCurrentNotebookSources() {
  if (!notebookId.value) {
    return;
  }
  await sourceStore.loadSources(notebookId.value);
  // 首次加载时自动打开第一个来源 tab，保证可见预览入口。
  if (sourceStore.items.length > 0 && openedSourceTabs.value.length === 0) {
    const firstID = sourceStore.items[0].id;
    openedSourceTabs.value = [firstID];
    activeSourceTabId.value = firstID;
    activeCenterTab.value = firstID;
    sourceStore.selectSource(firstID);
  }
}

/**
 * 加载当前笔记本笔记
 */
async function loadCurrentNotebookNotes() {
  if (!notebookId.value) {
    return;
  }
  await noteStore.loadNotes(notebookId.value);
}

/**
 * 加载当前笔记本会话
 */
async function loadCurrentNotebookChats() {
  if (!notebookId.value) {
    return;
  }
  await chatStore.loadSessions(notebookId.value);
  if (chatStore.activeSessionId) {
    await chatStore.loadSessionDetail(notebookId.value, chatStore.activeSessionId);
  }
}

/**
 * 提取纯文本预览
 */
function toPlainText(markdown: string): string {
  return markdown
    .replace(/^#+\s+/gm, "")
    .replace(/\*\*/g, "")
    .replace(/`/g, "")
    .replace(/\n+/g, " ")
    .trim();
}

/**
 * 格式化时间显示
 */
function formatDate(value?: string): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * 打开来源预览 tab
 */
function openSourceTab(sourceId: string) {
  if (!sourceId) {
    return;
  }
  if (!openedSourceTabs.value.includes(sourceId)) {
    openedSourceTabs.value.push(sourceId);
  }
  activeSourceTabId.value = sourceId;
  activeCenterTab.value = sourceId;
  sourceStore.selectSource(sourceId);
}

/**
 * 激活来源预览 tab
 */
function activateSourceTab(sourceId: string) {
  if (!sourceId) {
    return;
  }
  activeSourceTabId.value = sourceId;
  activeCenterTab.value = sourceId;
  sourceStore.selectSource(sourceId);
  textSearchInput.value = "";
  textSearchMatchCount.value = 0;
  textSearchCurrentIndex.value = -1;
  resetImagePreviewTransform();
}

/**
 * 关闭来源预览 tab
 */
function closeSourceTab(sourceId: string) {
  const nextTabs = openedSourceTabs.value.filter((id) => id !== sourceId);
  openedSourceTabs.value = nextTabs;
  if (activeSourceTabId.value === sourceId) {
    const fallbackId = nextTabs[nextTabs.length - 1] || "";
    activeSourceTabId.value = fallbackId;
    if (fallbackId) {
      sourceStore.selectSource(fallbackId);
    }
  }
  if (activeCenterTab.value === sourceId) {
    const fallbackTab = nextTabs[nextTabs.length - 1] || "chat";
    activeCenterTab.value = fallbackTab;
    if (fallbackTab !== "chat") {
      activeSourceTabId.value = fallbackTab;
      sourceStore.selectSource(fallbackTab);
    }
  }
  textSearchInput.value = "";
  textSearchMatchCount.value = 0;
  textSearchCurrentIndex.value = -1;
  resetImagePreviewTransform();
}

/**
 * 显示上一张 PPT 图片
 */
function showPrevPPTSlide() {
  if (pptPrevDisabled.value) {
    return;
  }
  pptSlideIndex.value -= 1;
}

/**
 * 显示下一张 PPT 图片
 */
function showNextPPTSlide() {
  if (pptNextDisabled.value) {
    return;
  }
  pptSlideIndex.value += 1;
}

/**
 * 截断来源 tab 标题
 */
function truncateSourceTabTitle(title: string): string {
  if (title.length <= 20) {
    return title;
  }
  return `${title.slice(0, 17)}...`;
}

/**
 * 切换左侧面板折叠状态
 */
function toggleLeftPanel() {
  leftCollapsed.value = !leftCollapsed.value;
}

/**
 * 切换右侧面板折叠状态
 */
function toggleRightPanel() {
  rightCollapsed.value = !rightCollapsed.value;
}

/**
 * 放大图片预览
 */
function zoomInImage() {
  imageZoom.value = Math.min(3, imageZoom.value + 0.1);
}

/**
 * 缩小图片预览
 */
function zoomOutImage() {
  imageZoom.value = Math.max(0.2, imageZoom.value - 0.1);
}

/**
 * 向左旋转图片预览
 */
function rotateImageLeft() {
  imageRotate.value -= 90;
}

/**
 * 向右旋转图片预览
 */
function rotateImageRight() {
  imageRotate.value += 90;
}

/**
 * 重置图片预览变换状态
 */
function resetImagePreviewTransform() {
  imageZoom.value = 1;
  imageRotate.value = 0;
}

/**
 * 获取 PDF.js 全局对象
 */
function getPdfJsLib(): PdfJsLike {
  const pdfjsLib = window.pdfjsLib;
  if (!pdfjsLib) {
    throw new Error("PDF.js 未加载");
  }
  return pdfjsLib;
}

/**
 * 生成文件预览请求头
 */
function buildFilePreviewHeaders(): HeadersInit {
  const token = localStorage.getItem("token");
  if (!token) {
    return {};
  }
  return {
    Authorization: `Bearer ${token}`,
  };
}

/**
 * 重置 PDF 预览状态
 */
function resetPdfPreviewState() {
  if (pdfDocument.value?.destroy) {
    pdfDocument.value.destroy();
  }
  pdfDocument.value = null;
  pdfLoading.value = false;
  pdfError.value = "";
  pdfCurrentPage.value = 1;
  pdfTotalPages.value = 0;
  pdfScale.value = 1;
  const canvas = pdfCanvasRef.value;
  const ctx = canvas?.getContext("2d");
  if (canvas && ctx) {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    canvas.width = 0;
    canvas.height = 0;
  }
}

/**
 * 渲染指定 PDF 页码
 */
async function renderPdfPage(pageNumber: number) {
  if (!pdfDocument.value) {
    return;
  }
  const canvas = pdfCanvasRef.value;
  const ctx = canvas?.getContext("2d");
  if (!canvas || !ctx) {
    return;
  }

  const page = await pdfDocument.value.getPage(pageNumber);
  const viewport = page.getViewport({ scale: pdfScale.value });

  canvas.width = viewport.width;
  canvas.height = viewport.height;
  pdfCurrentPage.value = pageNumber;

  await page.render({
    canvasContext: ctx,
    viewport,
  }).promise;
}

/**
 * 加载当前来源的 PDF 预览
 */
async function loadPdfPreview() {
  if (previewType.value !== "pdf" || !selectedSourceFileUrl.value) {
    resetPdfPreviewState();
    return;
  }

  const requestId = pdfRequestId.value + 1;
  pdfRequestId.value = requestId;
  resetPdfPreviewState();
  pdfLoading.value = true;
  pdfError.value = "";

  try {
    await nextTick();
    const pdfjsLib = getPdfJsLib();
    const response = await fetch(selectedSourceFileUrl.value, {
      headers: buildFilePreviewHeaders(),
    });
    if (!response.ok) {
      throw new Error(`加载 PDF 失败: ${response.status} ${response.statusText}`);
    }

    const pdfData = await response.arrayBuffer();
    const loadedDocument = await pdfjsLib.getDocument({ data: pdfData }).promise;

    if (pdfRequestId.value !== requestId) {
      loadedDocument.destroy?.();
      return;
    }

    pdfDocument.value = loadedDocument;
    pdfTotalPages.value = loadedDocument.numPages;
    pdfCurrentPage.value = 1;
    pdfScale.value = 1;
  } catch (error) {
    pdfError.value = error instanceof Error ? error.message : "PDF 预览加载失败";
  } finally {
    if (pdfRequestId.value === requestId) {
      pdfLoading.value = false;
    }
  }

  if (pdfRequestId.value !== requestId || !pdfDocument.value || pdfError.value) {
    return;
  }

  await nextTick();
  await renderPdfPage(1);
}

/**
 * 切换 PDF 页码
 */
async function changePdfPage(delta: number) {
  const nextPage = pdfCurrentPage.value + delta;
  if (!pdfDocument.value || nextPage < 1 || nextPage > pdfTotalPages.value) {
    return;
  }
  await renderPdfPage(nextPage);
}

/**
 * 调整 PDF 缩放比例
 */
async function zoomPdf(delta: number) {
  if (!pdfDocument.value) {
    return;
  }
  pdfScale.value = Math.max(0.25, Math.min(3, pdfScale.value + delta));
  await renderPdfPage(pdfCurrentPage.value);
}

/**
 * 按旧版逻辑让 PDF 适应容器宽度
 */
async function fitPdfWidth() {
  if (!pdfDocument.value || !pdfViewportRef.value) {
    return;
  }
  const page = await pdfDocument.value.getPage(pdfCurrentPage.value);
  const baseViewport = page.getViewport({ scale: 1 });
  pdfScale.value = Math.max(0.25, (pdfViewportRef.value.clientWidth - 40) / baseViewport.width);
  await renderPdfPage(pdfCurrentPage.value);
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
 * 渲染对话消息内容（助手消息走 Markdown，用户消息保留纯文本）
 */
function renderChatMessageHtml(role: string, content: string): string {
  const safeContent = typeof content === "string" ? content : "";
  if (role !== "assistant") {
    return `<p>${escapeHTML(safeContent).replace(/\n/g, "<br />")}</p>`;
  }
  const markedLib = window.marked;
  if (!markedLib) {
    return `<pre>${escapeHTML(safeContent)}</pre>`;
  }
  return markedLib.parse(safeContent);
}

/**
 * 按旧版规则清洗 Mermaid 思维导图源码
 */
function sanitizeMermaidCode(code: string): string {
  let sanitized = code.trim();

  // Handle graph LR/TB horizontal layouts
  if (/^graph\s+(LR|TB|RL|BT)\b/i.test(sanitized)) {
    // Fix arrow connections with parentheses in node labels: A(text) --> B
    sanitized = sanitized.replace(/(\s+-->)(\s+)([^"'\s][^-\n>]*\([^)]*\)[^-\n>]*)/g, '$1$2"$3"');
    sanitized = sanitized.replace(/(\s+---)(\s+)([^"'\s][^-\n>]*\([^)]*\)[^-\n>]*)/g, '$1$2"$3"');
    // Fix parentheses before arrow: text(abc) --> B becomes "text(abc)" -->
    sanitized = sanitized.replace(/([^"'\s][^-\n>]*\([^)]*\)[^-\n>]*)(\s+-->)(\s+)/g, '"$1"$2$3');
    sanitized = sanitized.replace(/([^"'\s][^-\n>]*\([^)]*\)[^-\n>]*)(\s+---)(\s+)/g, '"$1"$2$3');
    // Remove quotes from already-quoted content
    sanitized = sanitized.replace(/"([^"]*)"/g, '$1');
    return sanitized;
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
 * 初始化 Mermaid 配置
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
      primaryColor: "#818cf8",
      primaryTextColor: "#ffffff",
      primaryBorderColor: "#6366f1",
      lineColor: "#a5b4fc",
      secondaryColor: "#e0e7ff",
      tertiaryColor: "#ffffff",
      fontSize: "13px",
      mainBkg: "#818cf8",
      nodeBorder: "#818cf8",
      clusterBkg: "#e0e7ff",
      nodeTextColor: "#1e293b",
      edgeColor: "#94a3b8",
      nodePadding: "10, 15",
    },
    flowchart: {
      useMaxWidth: false,
      htmlLabels: true,
      curve: "basis",
      padding: 15,
    },
    mindmap: {
      useMaxWidth: true,
      padding: 20,
    },
  });
  mermaidInitialized = true;
}

/**
 * 将 Mermaid SVG 包装为 Markdown 风格容器，避免画布化展示
 */
function createMermaidMarkdownElement(svg: string) {
  const MIN_ZOOM = 0.1;
  const MAX_ZOOM = 1.0;
  const STEP_ZOOM = 0.1;
  const DEFAULT_ZOOM = 0.2;

  const container = document.createElement("div");
  container.className = "mermaid-svg-container";
  const controls = document.createElement("div");
  controls.className = "mermaid-zoom-controls";
  const zoomOutButton = document.createElement("button");
  zoomOutButton.type = "button";
  zoomOutButton.className = "mermaid-zoom-btn";
  zoomOutButton.textContent = "-";
  zoomOutButton.setAttribute("aria-label", "缩小思维导图");
  const zoomResetButton = document.createElement("button");
  zoomResetButton.type = "button";
  zoomResetButton.className = "mermaid-zoom-btn";
  zoomResetButton.textContent = "重置";
  zoomResetButton.setAttribute("aria-label", "重置思维导图缩放");
  const zoomInButton = document.createElement("button");
  zoomInButton.type = "button";
  zoomInButton.className = "mermaid-zoom-btn";
  zoomInButton.textContent = "+";
  zoomInButton.setAttribute("aria-label", "放大思维导图");
  const zoomLabel = document.createElement("span");
  zoomLabel.className = "mermaid-zoom-label";

  controls.append(zoomOutButton, zoomResetButton, zoomInButton, zoomLabel);

  const viewport = document.createElement("div");
  viewport.className = "mermaid-svg-viewport";
  const stage = document.createElement("div");
  stage.className = "mermaid-svg-stage";
  stage.innerHTML = svg;
  viewport.appendChild(stage);
  container.append(controls, viewport);

  const svgElement = stage.querySelector("svg");
  if (svgElement instanceof SVGSVGElement) {
    svgElement.removeAttribute("width");
    svgElement.removeAttribute("height");
    svgElement.setAttribute("preserveAspectRatio", "xMidYMid meet");
  }

  let currentZoom = DEFAULT_ZOOM;
  const applyZoom = () => {
    const percent = Math.round(currentZoom * 100);
    stage.style.width = `${percent}%`;
    zoomLabel.textContent = `${percent}%`;
    zoomOutButton.disabled = currentZoom <= MIN_ZOOM;
    zoomInButton.disabled = currentZoom >= MAX_ZOOM;
  };
  const updateZoom = (nextZoom: number) => {
    currentZoom = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, Number(nextZoom.toFixed(2))));
    applyZoom();
  };

  zoomOutButton.addEventListener("click", () => {
    updateZoom(currentZoom - STEP_ZOOM);
  });
  zoomInButton.addEventListener("click", () => {
    updateZoom(currentZoom + STEP_ZOOM);
  });
  zoomResetButton.addEventListener("click", () => {
    updateZoom(DEFAULT_ZOOM);
  });
  applyZoom();

  return container;
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
    const renderId = `mermaid-diag-${Date.now()}-${index}`;

    try {
      const { svg } = await window.mermaid.render(renderId, cleanCode);
      const markdownElement = createMermaidMarkdownElement(svg);
      pre.replaceWith(markdownElement);
    } catch (error) {
      try {
        const { svg } = await window.mermaid.render(`${renderId}-retry`, cleanCode.replace(/\(|\)/g, ""));
        const markdownElement = createMermaidMarkdownElement(svg);
        pre.replaceWith(markdownElement);
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
 * 在笔记详情中渲染 Mermaid 思维导图
 */
async function renderSelectedNoteMermaid() {
  if (activeCenterTab.value !== "note" || !shouldRenderRichNote.value) {
    return;
  }
  await nextTick();
  await Promise.all([
    renderMermaidDiagrams(noteContentRef.value),
    renderMermaidDiagrams(sidebarNoteContentRef.value),
  ]);
}

/**
 * 转义正则特殊字符
 */
function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * 同步文本搜索匹配状态
 */
function syncTextSearchMatches() {
  const query = textSearchInput.value.trim();
  const content = markdownPreviewText.value || "";
  if (!query || !content) {
    textSearchMatchCount.value = 0;
    textSearchCurrentIndex.value = -1;
    return;
  }
  const regex = new RegExp(escapeRegExp(query), "gi");
  const matches = content.match(regex);
  textSearchMatchCount.value = matches?.length || 0;
  if (textSearchMatchCount.value <= 0) {
    textSearchCurrentIndex.value = -1;
    return;
  }
  if (textSearchCurrentIndex.value < 0 || textSearchCurrentIndex.value >= textSearchMatchCount.value) {
    textSearchCurrentIndex.value = 0;
  }
}

/**
 * 滚动到当前高亮匹配项
 */
async function scrollToActiveTextMatch() {
  await nextTick();
  const activeNode = document.querySelector(".text-content .search-highlight.active");
  if (activeNode instanceof HTMLElement) {
    activeNode.scrollIntoView({ behavior: "smooth", block: "center" });
  }
}

/**
 * 切换到上一个匹配项
 */
function focusPrevTextMatch() {
  if (textSearchMatchCount.value <= 0) {
    return;
  }
  textSearchCurrentIndex.value =
    (textSearchCurrentIndex.value - 1 + textSearchMatchCount.value) % textSearchMatchCount.value;
  void scrollToActiveTextMatch();
}

/**
 * 切换到下一个匹配项
 */
function focusNextTextMatch() {
  if (textSearchMatchCount.value <= 0) {
    return;
  }
  textSearchCurrentIndex.value = (textSearchCurrentIndex.value + 1) % textSearchMatchCount.value;
  void scrollToActiveTextMatch();
}

/**
 * 清空文本搜索
 */
function clearTextSearch() {
  textSearchInput.value = "";
}

/**
 * 加载信息图风格列表
 */
async function loadInfographStyles() {
  if (infographStylesLoaded.value) {
    return;
  }
  try {
    const response = await request<{ styles?: Array<{ id: string; name: string; description: string }> }>(
      "/v2/infograph/styles",
    );
    infographStyles.value = response.styles || [];
  } catch {
    infographStyles.value = [
      { id: "craft-handmade", name: "手绘纸艺风格", description: "手绘和纸艺美学，温暖有机的感觉" },
      { id: "cyberpunk-neon", name: "赛博朋克霓虹风格", description: "深色背景上的霓虹发光效果，未来主义美学" },
      { id: "kawaii", name: "日系可爱风格", description: "日式可爱风格，大眼睛和柔和色调" },
      { id: "technical-schematic", name: "工程蓝图风格", description: "工程技术图纸，精确的几何线条" },
    ];
  } finally {
    infographStylesLoaded.value = true;
  }
}

/**
 * 切换信息图风格面板
 */
async function toggleInfographStyleDropdown() {
  if (infographStyleDropdownVisible.value) {
    infographStyleDropdownVisible.value = false;
    return;
  }
  await loadInfographStyles();
  infographStyleDropdownVisible.value = true;
}

/**
 * 选择信息图风格并自动执行生成
 */
async function selectInfographStyle(styleId: string) {
  selectedInfographStyle.value = styleId || null;
  infographStyleDropdownVisible.value = false;
  await runTransform("infograph");
}

/**
 * 切换到笔记详情视图
 */
function openNoteDetail(noteId: string) {
  noteStore.selectNote(noteId);
  activeNoteView.value = "note_detail";
  activeCenterTab.value = "note";
}

/**
 * 应用预设提示场景
 */
function applyPromptScenario(prompt: string) {
  chatInput.value = prompt;
}

/**
 * 切换快捷场景折叠状态
 */
function togglePromptScenariosPanel() {
  promptScenariosCollapsed.value = !promptScenariosCollapsed.value;
}

/**
 * 获取快捷场景图标（与旧版保持一致）
 */
function getPromptScenarioIcon(name: string): string {
  const icons: Record<string, string> = {
    search:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.35-4.35"></path></svg>',
    question:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><path d="M12 17h.01"></path></svg>',
    lightbulb:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18h6"></path><path d="M10 22h4"></path><path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 12 3.5a4.65 4.65 0 0 0-4.5 4.5v1.29c0 .75-.15 1.48-.5 2.11"></path></svg>',
    compare:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="18"></rect><rect x="14" y="3" width="7" height="18"></rect><path d="M10 9h4"></path><path d="M10 15h4"></path></svg>',
    action:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3zM7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3"></path></svg>',
    detail:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.35-4.35"></path><path d="M11 8v6"></path><path d="M8 11h6"></path></svg>',
  };
  return icons[name] || icons.search;
}

/**
 * 关闭笔记详情 tab
 */
function closeActiveNoteTab() {
  noteStore.selectNote("");
  activeCenterTab.value = "chat";
}

/**
 * 复制当前笔记 Markdown 内容
 */
async function copySelectedNoteMarkdown() {
  if (!selectedNote.value?.content) {
    return;
  }
  await navigator.clipboard.writeText(selectedNote.value.content);
  // 使用浏览器原生通知或 alert 替代未引入的 toast
  if (window.Notification && Notification.permission === "granted") {
    new Notification("复制成功");
  } else {
    alert("复制成功");
  }
}

/**
 * 返回首页
 */
function backToList() {
  router.push("/");
}

/**
 * 创建新笔记本并跳转
 */
async function createNotebook() {
  const name = window.prompt("请输入笔记本名称");
  if (!name) {
    return;
  }
  try {
    const notebook = await notebookStore.createNotebook(name);
    await router.push(`/notes/${notebook.id}`);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    window.alert(`创建失败：${message}`);
  }
}

/**
 * 打开添加来源弹层
 */
function openAddSourceModal() {
  addSourceModalVisible.value = true;
  addSourceTab.value = "file";
}

/**
 * 关闭添加来源弹层
 */
function closeAddSourceModal() {
  addSourceModalVisible.value = false;
  addSourceTab.value = "file";
  textSourceName.value = "";
  textSourceContent.value = "";
  urlSourceName.value = "";
  urlSourceValue.value = "";
}

/**
 * 处理弹层中的文件上传
 */
async function onModalFileChange(event: Event) {
  const target = event.target as HTMLInputElement;
  const files = target.files;
  if (!files || files.length === 0 || !notebookId.value) {
    return;
  }
  await sourceStore.uploadFile(notebookId.value, files[0]);
  target.value = "";
  closeAddSourceModal();
}

/**
 * 添加文本来源
 */
async function submitTextSource() {
  if (!notebookId.value) {
    return;
  }
  try {
    await sourceStore.createTextSource(notebookId.value, textSourceName.value, textSourceContent.value);
    closeAddSourceModal();
  } catch {
    // 错误由 sourceStore.error 承载
  }
}

/**
 * 添加 URL 来源
 */
async function submitURLSource() {
  if (!notebookId.value) {
    return;
  }
  try {
    await sourceStore.createURLSource(notebookId.value, urlSourceValue.value, urlSourceName.value);
    closeAddSourceModal();
  } catch {
    // 错误由 sourceStore.error 承载
  }
}

/**
 * 发送对话消息
 */
async function sendMessage() {
  if (!notebookId.value) {
    return;
  }
  const text = chatInput.value.trim();
  if (!text) {
    return;
  }
  chatInput.value = "";
  await chatStore.sendMessage(notebookId.value, text);
}

/**
 * 执行基础类型转换
 */
async function runTransform(type: TransformType) {
  if (!notebookId.value) {
    return;
  }
  const sourceIds = sources.value.map((item) => item.id);
  if (sourceIds.length === 0) {
    noteStore.error = "请先添加来源";
    return;
  }
  try {
    await noteStore.transformNote(
      notebookId.value,
      type,
      sourceIds,
      undefined,
      type === "infograph" ? { infographStyleId: selectedInfographStyle.value } : undefined,
    );
  } catch {
    // 错误消息由 noteStore.error 承载并在界面展示
  }
}

/**
 * 执行自定义提示词转换
 */
async function runCustomTransform() {
  const prompt = customPrompt.value.trim();
  if (!prompt) {
    noteStore.error = "请输入自定义提示词";
    return;
  }
  if (!notebookId.value) {
    return;
  }
  const sourceIds = sources.value.map((item) => item.id);
  if (sourceIds.length === 0) {
    noteStore.error = "请先添加来源";
    return;
  }
  try {
    await noteStore.transformNote(notebookId.value, "custom", sourceIds, prompt);
    customPrompt.value = "";
  } catch {
    // 错误消息由 noteStore.error 承载并在界面展示
  }
}

/**
 * 删除笔记
 */
async function deleteNote(noteId: string) {
  if (!notebookId.value || !noteId) {
    return;
  }
  const confirmed = window.confirm("确定删除该笔记吗？");
  if (!confirmed) {
    return;
  }
  try {
    await noteStore.deleteNote(notebookId.value, noteId);
  } catch {
    // 错误消息由 noteStore.error 承载并在界面展示
  }
}

/**
 * 创建新会话
 */
async function createSession() {
  if (!notebookId.value) {
    return;
  }
  await chatStore.createSession(notebookId.value);
  if (chatStore.activeSessionId) {
    await chatStore.loadSessionDetail(notebookId.value, chatStore.activeSessionId);
  }
}

/**
 * 切换会话
 */
async function openSession(sessionId: string) {
  if (!notebookId.value || !sessionId) {
    return;
  }
  // 与旧版行为保持一致：从会话历史点击后直接进入对话视图。
  activeCenterTab.value = "chat";
  await chatStore.loadSessionDetail(notebookId.value, sessionId);
}

/**
 * 删除单个会话
 */
async function deleteSession(sessionId: string) {
  if (!notebookId.value || !sessionId) {
    return;
  }
  const confirmed = window.confirm("确定要删除这个对话吗？");
  if (!confirmed) {
    return;
  }
  try {
    await chatStore.deleteSession(notebookId.value, sessionId);
  } catch {
    // 错误消息由 chatStore.error 承载并在界面展示
  }
}

/**
 * 清空所有会话
 */
async function clearSessions() {
  if (!notebookId.value) {
    return;
  }
  if (sessions.value.length === 0) {
    chatStore.error = "暂无对话历史";
    return;
  }
  const confirmed = window.confirm(
    `确定要清空所有对话历史吗？这将删除 ${sessions.value.length} 个对话。`,
  );
  if (!confirmed) {
    return;
  }
  try {
    await chatStore.clearSessions(notebookId.value);
  } catch {
    // 错误消息由 chatStore.error 承载并在界面展示
  }
}

/**
 * 获取会话摘要（优先顶层 summary，兜底 metadata.summary）
 */
function getSessionSummary(session: ChatSession): string {
  const direct = session.summary?.trim();
  if (direct) {
    return direct;
  }
  const metadataSummary = session.metadata?.summary;
  if (typeof metadataSummary === "string") {
    return metadataSummary.trim();
  }
  return "";
}

/**
 * 获取会话列表的副文本（优先摘要，兜底显示更新时间）
 */
function getSessionMetaText(session: ChatSession): string {
  const summary = getSessionSummary(session);
  if (summary) {
    return summary;
  }
  return formatDate(session.updated_at || session.created_at);
}

/**
 * 启动来源状态轮询
 */
function startSourcePolling() {
  if (!notebookId.value) {
    return;
  }
  if (pollingTimer) {
    clearInterval(pollingTimer);
  }
  pollingTimer = setInterval(() => {
    void sourceStore.refreshProcessingStatuses(notebookId.value);
  }, 3000);
}

/**
 * 停止来源状态轮询
 */
function stopSourcePolling() {
  if (pollingTimer) {
    clearInterval(pollingTimer);
    pollingTimer = null;
  }
}

/**
 * 重新加载工作区数据
 */
async function reloadWorkspaceData() {
  if (!notebookId.value) {
    return;
  }
  chatStore.reset();
  await Promise.all([
    notebookStore.loadNotebooks(),
    loadCurrentNotebookSources(),
    loadCurrentNotebookNotes(),
    loadCurrentNotebookChats(),
  ]);
  activeNoteView.value = "notes_list";
  startSourcePolling();
}

onMounted(async () => {
  documentClickHandler = (event: MouseEvent) => {
    if (!infographStyleDropdownVisible.value) {
      return;
    }
    const target = event.target as HTMLElement | null;
    if (!target) {
      return;
    }
    const inDropdown = target.closest("#infographStyleDropdown");
    const inPicker = target.closest(".style-picker-btn");
    if (!inDropdown && !inPicker) {
      infographStyleDropdownVisible.value = false;
    }
  };
  document.addEventListener("click", documentClickHandler);
  window.addEventListener("resize", handleChartResize);
  await reloadWorkspaceData();
});

watch(
  () => notebookId.value,
  async () => {
    stopSourcePolling();
    await reloadWorkspaceData();
  },
);

watch(
  () => textSearchInput.value,
  () => {
    syncTextSearchMatches();
    if (textSearchMatchCount.value > 0) {
      void scrollToActiveTextMatch();
    }
  },
);

watch(
  () => markdownPreviewText.value,
  () => {
    syncTextSearchMatches();
  },
);

watch(
  () => [previewType.value, selectedSourceFileUrl.value, activeCenterTab.value],
  () => {
    void loadPdfPreview();
  },
);

watch(
  () => [selectedNote.value?.id, selectedNotePPTSlides.value.length],
  () => {
    pptSlideIndex.value = 0;
  },
);

watch(
  () => [selectedNote.value?.id, selectedNote.value?.content, activeCenterTab.value],
  () => {
    void renderSelectedNoteCharts();
    void renderSelectedNoteMermaid();
  },
);

onBeforeUnmount(() => {
  if (documentClickHandler) {
    document.removeEventListener("click", documentClickHandler);
    documentClickHandler = null;
  }
  window.removeEventListener("resize", handleChartResize);
  stopSourcePolling();
  disposeChartInstances();
  resetPdfPreviewState();
});
</script>

<template>
  <main class="app-container">
    <div class="workspace-container">
      <nav class="workspace-nav">
        <button class="btn-back" @click="backToList">
          <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="15" y1="9" x2="3" y2="9" />
            <polyline points="9,15 3,9 9,3" />
          </svg>
          返回列表
        </button>
        <div class="current-notebook-info">
          <h2 class="notebook-name-display">{{ currentNotebook?.name || `笔记本 ${notebookId}` }}</h2>
        </div>
        <div class="workspace-actions">
          <button class="btn-new-notebook" @click="createNotebook">新建笔记本</button>
        </div>
      </nav>

      <main class="main-grid" :class="{ 'left-collapsed': leftCollapsed, 'right-collapsed': rightCollapsed }">
        <aside class="panel panel-left">
          <div class="panel-header">
            <h2 class="panel-title">来源</h2>
            <div class="panel-actions">
              <button class="btn-icon" title="添加来源" @click="openAddSourceModal">
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="9" y1="2" x2="9" y2="16" />
                  <line x1="2" y1="9" x2="16" y2="9" />
                </svg>
              </button>
              <button class="btn-toggle-side" title="切换侧边栏" @click="toggleLeftPanel">
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M10 3L5 8L10 13" />
                </svg>
              </button>
            </div>
          </div>

          <p v-if="loading" class="muted">来源加载中...</p>
          <p v-else-if="error" class="error">来源加载失败：{{ error }}</p>
          <ul v-else class="source-list">
            <li
              v-for="item in sources"
              :key="item.id"
              class="source-item"
              :class="{ active: selectedSource?.id === item.id }"
              @click="openSourceTab(item.id)"
            >
              <div class="title">{{ item.name }}</div>
              <div class="meta">
                {{ item.type }} · {{ formatSize(item.file_size) }} · {{ (item.status && item.status.trim()) || "processing" }}
              </div>
            </li>
          </ul>

        </aside>

        <div class="resizer" id="resizerLeft"></div>

        <section class="panel panel-center">
          <div class="panel-header">
            <div class="panel-tabs">
              <button
                class="tab-btn"
                :class="{ active: activeCenterTab === 'chat' }"
                @click="activeCenterTab = 'chat'"
              >
                对话
              </button>
              <button
                class="tab-btn"
                :class="{ active: activeCenterTab === 'sessions' }"
                @click="activeCenterTab = 'sessions'"
              >
                会话历史
              </button>
              <button
                class="tab-btn"
                :class="{ active: activeCenterTab === 'notes_list' }"
                @click="activeCenterTab = 'notes_list'"
              >
                笔记列表
              </button>
              <button
                v-for="tab in sourceTabs"
                :key="tab.id"
                class="tab-btn"
                :class="{ active: activeCenterTab === tab.id }"
                :data-tab="tab.id"
                @click="activateSourceTab(tab.id)"
              >
                <span class="tab-title">{{ truncateSourceTabTitle(tab.name) }}</span>
              </button>
              <button
                class="tab-btn"
                :class="{ active: activeCenterTab === 'note' }"
                v-show="Boolean(selectedNote)"
                @click="activeCenterTab = 'note'"
              >
                笔记
                <span class="tab-close" @click.stop="closeActiveNoteTab">×</span>
              </button>
            </div>
            <div class="panel-header-actions">
              <!-- <button class="btn-icon" title="新建对话" :disabled="clearingSessions" @click="createSession">
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="8" y1="2" x2="8" y2="14" />
                  <line x1="2" y1="8" x2="14" y2="8" />
                </svg>
              </button>
              <button
                v-if="activeCenterTab !== 'sessions'"
                class="btn-icon"
                title="清空历史"
                :disabled="clearingSessions"
                @click="clearSessions"
              >
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M3 6h10M8 6v7M5 6l-2 6M11 6l2 6" />
                </svg>
              </button> -->
            </div>
          </div>

          <div v-if="activeCenterTab === 'sessions'" class="chat-sessions-panel">
            <div class="sessions-header">
              <div class="sessions-header-left">
                <h3>对话历史</h3>
                <p class="sessions-subtitle">记住一下对话，省时又省力</p>
              </div>
              <button class="btn-clear-sessions" :disabled="clearingSessions" @click="clearSessions">清空</button>
            </div>
            <p v-if="loadingSessions" class="muted">会话加载中...</p>
            <ul v-else class="sessions-list">
              <li
                v-for="session in sessions"
                :key="session.id"
                class="chat-session-item"
                :class="{ active: activeSessionId === session.id }"
                @click="openSession(session.id)"
              >
                <div class="session-content">
                  <div class="session-title">{{ session.title || "新会话" }}</div>
                  <div class="session-time">{{ getSessionMetaText(session) }}</div>
                </div>
                <button
                  class="btn-delete-session"
                  :disabled="clearingSessions || deletingSessionId === session.id"
                  title="删除会话"
                  @click.stop="deleteSession(session.id)"
                >
                  <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M4.5 4.5L9.5 9.5M9.5 4.5L4.5 9.5"></path>
                  </svg>
                </button>
              </li>
            </ul>
          </div>

          <div v-else-if="activeCenterTab === 'notes_list'" class="chat-sessions-panel">
            <div class="notes-compact-grid">
              <div
                v-for="item in notes"
                :key="item.id"
                class="compact-note-card"
                :data-note-id="item.id"
                @click="openNoteDetail(item.id)"
              >
                <button class="btn-delete-compact-note" title="删除笔记" @click.stop="deleteNote(item.id)">
                  <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M4.5 4.5L9.5 9.5M9.5 4.5L4.5 9.5"></path>
                  </svg>
                </button>
                <div class="note-type">{{ item.type }}</div>
                <h4 class="note-title">{{ item.title }}</h4>
                <p class="note-preview">{{ toPlainText(item.content) }}</p>
                <div class="note-footer">
                  <span>{{ formatDate(item.created_at) }}</span>
                  <span>{{ item.source_ids?.length || 0 }} 来源</span>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeCenterTab === 'note'" class="chat-panel">
            <div v-if="selectedNote" class="note-view-container" style="display: flex">
              <div class="note-view-header">
                <div class="note-view-info">
                  <span class="note-view-type">{{ selectedNote.type }}</span>
                  <span class="note-view-title-text">{{ selectedNote.title }}</span>
                </div>
                <div class="note-view-actions">
                  <button class="btn-copy-note" title="复制 Markdown" @click="copySelectedNoteMarkdown">
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                      <rect x="3" y="3" width="10" height="10" rx="1" />
                      <path d="M7 3 L7 1 C7 1 13 1 13 1 L13 13 L11 13" />
                    </svg>
                  </button>
                </div>
              </div>
              <div ref="noteContentRef" class="note-view-content">
                <div v-if="shouldRenderDataChart" class="charts-container">
                  <div
                    v-for="(chart, index) in parsedChartItems"
                    :key="`${selectedNote.id}-${index}`"
                    class="chart-wrapper"
                  >
                    <div v-if="chart.title" class="chart-title">{{ chart.title }}</div>
                    <div :ref="(element) => setChartContainerRef(element, index)" class="chart-div"></div>
                  </div>
                </div>
                <div
                  v-else-if="shouldRenderRichNote"
                  class="markdown-content"
                  v-html="selectedNoteRenderedHtml"
                ></div>
                <div v-else-if="shouldRenderPPTSlides" class="ppt-slides-view">
                  <div class="ppt-slides-toolbar">
                    <button type="button" class="btn-slide-nav" :disabled="pptPrevDisabled" @click="showPrevPPTSlide">
                      上一张
                    </button>
                    <span class="ppt-slides-indicator">{{ pptSlideIndicator }}</span>
                    <button type="button" class="btn-slide-nav" :disabled="pptNextDisabled" @click="showNextPPTSlide">
                      下一张
                    </button>
                  </div>
                  <div class="ppt-slide-image-wrap">
                    <img :src="currentPPTSlideUrl" :alt="`幻灯片 ${pptSlideIndicator}`" loading="lazy" />
                  </div>
                </div>
                <div v-else-if="selectedNoteInfographImageUrl" class="markdown-content">
                  <img :src="selectedNoteInfographImageUrl" alt="信息图" loading="lazy" />
                </div>
                <div v-else class="markdown-content">
                  <pre>{{ selectedNote.content }}</pre>
                </div>
              </div>
            </div>
            <p v-else class="muted">暂无可展示笔记</p>
          </div>

          <div v-else-if="activeSourceTabForCenter" class="resource-preview-container">
            <div class="resource-preview-header">
              <h3 class="resource-title">{{ activeSourceTabForCenter.name || "资源预览" }}</h3>
              <div class="resource-actions">
                <button class="btn-resource-action" title="关闭" @click="closeSourceTab(activeSourceTabForCenter.id)">
                  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="4" y1="4" x2="12" y2="12" />
                    <line x1="12" y1="4" x2="4" y2="12" />
                  </svg>
                </button>
              </div>
            </div>
            <div class="resource-preview-content">
              <div v-if="previewType === 'image'" class="image-preview">
                <div class="image-toolbar">
                  <button class="btn-image-zoom-in" title="放大" @click="zoomInImage">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8"></circle>
                      <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                      <line x1="11" y1="8" x2="11" y2="14"></line>
                      <line x1="8" y1="11" x2="14" y2="11"></line>
                    </svg>
                  </button>
                  <button class="btn-image-zoom-out" title="缩小" @click="zoomOutImage">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8"></circle>
                      <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                      <line x1="8" y1="11" x2="14" y2="11"></line>
                    </svg>
                  </button>
                  <button class="btn-image-reset" title="重置" @click="resetImagePreviewTransform">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"></path>
                      <path d="M3 3v5h5"></path>
                    </svg>
                  </button>
                  <div class="separator"></div>
                  <button class="btn-image-rotate-left" title="向左旋转" @click="rotateImageLeft">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"></path>
                      <path d="M3 3v5h5"></path>
                    </svg>
                  </button>
                  <button class="btn-image-rotate-right" title="向右旋转" @click="rotateImageRight">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M21 12a9 9 0 1 1-9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"></path>
                      <path d="M21 3v5h-5"></path>
                    </svg>
                  </button>
                  <span class="zoom-level">{{ Math.round(imageZoom * 100) }}%</span>
                </div>
                <div class="image-viewport">
                  <img
                    class="image-preview-img"
                    :src="selectedSourceFileUrl"
                    :alt="activeSourceTabForCenter.name"
                    :style="imageTransformStyle"
                  />
                </div>
              </div>
              <div v-else-if="previewType === 'pdf'" class="resource-body">
                <div v-if="pdfLoading" class="resource-loading">
                  <div class="spinner"></div>
                  <div>加载 PDF 中...</div>
                </div>
                <div v-else-if="pdfError" class="resource-error">
                  <div class="error-message">{{ pdfError }}</div>
                  <button class="btn-retry" @click="loadPdfPreview">重试</button>
                </div>
                <div v-else class="pdf-preview">
                  <div class="pdf-toolbar">
                    <button class="btn-pdf-prev" title="上一页" :disabled="pdfPrevDisabled" @click="changePdfPage(-1)">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="15 18 9 12 15 6"></polyline>
                      </svg>
                    </button>
                    <span class="pdf-page-info">
                      <span class="current-page">{{ pdfCurrentPage }}</span> /
                      <span class="total-pages">{{ pdfTotalPages || "-" }}</span>
                    </span>
                    <button class="btn-pdf-next" title="下一页" :disabled="pdfNextDisabled" @click="changePdfPage(1)">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="9 18 15 12 9 6"></polyline>
                      </svg>
                    </button>
                    <div class="separator"></div>
                    <button class="btn-pdf-zoom-out" title="缩小" @click="zoomPdf(-0.2)">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="11" cy="11" r="8"></circle>
                        <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                        <line x1="8" y1="11" x2="14" y2="11"></line>
                      </svg>
                    </button>
                    <span class="pdf-zoom-level">{{ pdfZoomLevelLabel }}</span>
                    <button class="btn-pdf-zoom-in" title="放大" @click="zoomPdf(0.2)">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="11" cy="11" r="8"></circle>
                        <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                        <line x1="11" y1="8" x2="11" y2="14"></line>
                        <line x1="8" y1="11" x2="14" y2="11"></line>
                      </svg>
                    </button>
                    <div class="separator"></div>
                    <button class="btn-pdf-fit-width" title="适应宽度" @click="fitPdfWidth">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <rect x="3" y="3" width="18" height="18" rx="2"></rect>
                        <line x1="9" y1="3" x2="9" y2="21"></line>
                        <line x1="15" y1="3" x2="15" y2="21"></line>
                      </svg>
                    </button>
                  </div>
                  <div ref="pdfViewportRef" class="pdf-viewport">
                    <div class="pdf-page-container">
                      <canvas ref="pdfCanvasRef" class="pdf-canvas"></canvas>
                    </div>
                  </div>
                </div>
              </div>
              <div v-else-if="previewType === 'markdown'" class="text-preview">
                <div class="text-search-bar">
                  <input
                    v-model="textSearchInput"
                    type="text"
                    class="text-search-input"
                    placeholder="搜索文本..."
                  />
                  <button
                    class="btn-search-prev"
                    title="上一个"
                    :disabled="textSearchMatchCount <= 0"
                    @click="focusPrevTextMatch"
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                      <polyline points="10 4 6 8 10 12"></polyline>
                    </svg>
                  </button>
                  <button
                    class="btn-search-next"
                    title="下一个"
                    :disabled="textSearchMatchCount <= 0"
                    @click="focusNextTextMatch"
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                      <polyline points="6 4 10 8 6 12"></polyline>
                    </svg>
                  </button>
                  <span class="search-count">{{ textSearchCountLabel }}</span>
                  <button class="btn-search-clear" title="清除" @click="clearTextSearch">
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                      <line x1="4" y1="4" x2="12" y2="12"></line>
                      <line x1="12" y1="4" x2="4" y2="12"></line>
                    </svg>
                  </button>
                </div>
                <div class="text-content" v-html="highlightedTextPreviewHtml"></div>
              </div>
              <p v-else class="muted">当前类型暂不支持内嵌预览。</p>
            </div>
          </div>

          <div v-else class="chat-panel">
            <div class="chat-messages">
              <div v-if="messages.length === 0" class="chat-welcome">
                <h3>开始对话</h3>
                <p>输入问题后，助手会基于当前来源内容给出回答。</p>
              </div>
              <div v-for="msg in messages" v-else :key="msg.id" class="chat-message" :data-role="msg.role">
                <div class="message-avatar">{{ msg.role === "user" ? "我" : "AI" }}</div>
                <div class="message-content">
                  <div class="message-text" v-html="renderChatMessageHtml(msg.role, msg.content)"></div>
                </div>
              </div>
            </div>
            <div class="prompt-scenarios-panel" :class="{ collapsed: promptScenariosCollapsed }">
              <div class="prompt-scenarios-header" @click="togglePromptScenariosPanel">
                <span class="prompt-scenarios-title">快捷场景</span>
                <svg
                  class="chevron-icon"
                  :class="{ rotated: promptScenariosCollapsed }"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <polyline points="6,9 12,15 18,9" />
                </svg>
              </div>
              <div class="prompt-scenarios-container">
                <button
                  v-for="scenario in promptScenarios"
                  :key="scenario.label"
                  class="prompt-scenario-btn"
                  @click="applyPromptScenario(scenario.prompt)"
                >
                  <span class="prompt-scenario-icon" v-html="getPromptScenarioIcon(scenario.icon)"></span>
                  <span class="prompt-scenario-text">{{ scenario.label }}</span>
                </button>
              </div>
            </div>
            <p v-if="chatError" class="error">{{ chatError }}</p>
            <div class="chat-input-wrapper">
              <form class="chat-form" @submit.prevent="sendMessage">
                <input
                  v-model="chatInput"
                  type="text"
                  class="chat-input"
                  placeholder="输入问题..."
                  autocomplete="off"
                />
                <button type="submit" class="btn-send" :disabled="sending">
                  <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="2" y1="16" x2="16" y2="2" />
                    <polyline points="2,2 2,16 16,16" />
                  </svg>
                </button>
              </form>
            </div>
          </div>
        </section>

        <div class="resizer" id="resizerRight"></div>

        <aside class="panel panel-right">
          <div class="workspace-section section-transform">
            <div class="panel-header studio-header">
              <button class="btn-toggle-side" title="切换侧边栏" @click="toggleRightPanel">
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 3L11 8L6 13" />
                </svg>
              </button>
              <h2 class="panel-title">STUDIO</h2>
            </div>
            <div class="transform-options">
            <div class="transform-grid">
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'summary' }"
                data-type="summary"
                :disabled="Boolean(transformingType)"
                @click="runTransform('summary')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>
                </span>
                <span class="transform-name">摘要</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'faq' }"
                data-type="faq"
                :disabled="Boolean(transformingType)"
                @click="runTransform('faq')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                </span>
                <span class="transform-name">常见问题</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'study_guide' }"
                data-type="study_guide"
                :disabled="Boolean(transformingType)"
                @click="runTransform('study_guide')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"></path><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"></path></svg>
                </span>
                <span class="transform-name">学习指南</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'outline' }"
                data-type="outline"
                :disabled="Boolean(transformingType)"
                @click="runTransform('outline')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"></line><line x1="8" y1="12" x2="21" y2="12"></line><line x1="8" y1="18" x2="21" y2="18"></line><line x1="3" y1="6" x2="3.01" y2="6"></line><line x1="3" y1="12" x2="3.01" y2="12"></line><line x1="3" y1="18" x2="3.01" y2="18"></line></svg>
                </span>
                <span class="transform-name">大纲</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'podcast' }"
                data-type="podcast"
                :disabled="Boolean(transformingType)"
                @click="runTransform('podcast')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"></path><path d="M19 10v2a7 7 0 0 1-14 0v-2"></path><line x1="12" y1="19" x2="12" y2="23"></line><line x1="8" y1="23" x2="16" y2="23"></line></svg>
                </span>
                <span class="transform-name">播客</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'timeline' }"
                data-type="timeline"
                :disabled="Boolean(transformingType)"
                @click="runTransform('timeline')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
                </span>
                <span class="transform-name">时间线</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'glossary' }"
                data-type="glossary"
                :disabled="Boolean(transformingType)"
                @click="runTransform('glossary')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"></path><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"></path></svg>
                </span>
                <span class="transform-name">术语表</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'quiz' }"
                data-type="quiz"
                :disabled="Boolean(transformingType)"
                @click="runTransform('quiz')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>
                </span>
                <span class="transform-name">测验</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'mindmap' }"
                data-type="mindmap"
                :disabled="Boolean(transformingType)"
                @click="runTransform('mindmap')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="5" r="3"></circle><circle cx="6" cy="12" r="3"></circle><circle cx="18" cy="19" r="3"></circle><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"></line><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"></line></svg>
                </span>
                <span class="transform-name">思维导图</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'infograph' }"
                data-type="infograph"
                :disabled="Boolean(transformingType)"
                @click="runTransform('infograph')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"></rect><path d="M21 12H3"></path><path d="M12 3v18"></path><path d="M16 8l-8 8"></path><path d="M8 8l8 8"></path></svg>
                </span>
                <span class="transform-name-row">
                  <span class="transform-name">信息图</span>
                  <span class="style-picker-btn" :title="infographStylePickerTitle" @click.stop="toggleInfographStyleDropdown">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"></path>
                    </svg>
                  </span>
                </span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'ppt' }"
                data-type="ppt"
                :disabled="Boolean(transformingType)"
                @click="runTransform('ppt')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>
                </span>
                <span class="transform-name">幻灯片</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'insight' }"
                data-type="insight"
                :disabled="Boolean(transformingType)"
                @click="runTransform('insight')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line><path d="M12 2L12 12"></path><path d="M12 12L16 16"></path></svg>
                </span>
                <span class="transform-name">洞察</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'data_table' }"
                data-type="data_table"
                :disabled="Boolean(transformingType)"
                @click="runTransform('data_table')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"></rect><line x1="3" y1="9" x2="21" y2="9"></line><line x1="3" y1="15" x2="21" y2="15"></line><line x1="9" y1="3" x2="9" y2="21"></line><line x1="15" y1="3" x2="15" y2="21"></line></svg>
                </span>
                <span class="transform-name">数据表格</span>
              </button>
              <button
                class="transform-card"
                :class="{ loading: transformingType === 'data_chart' }"
                data-type="data_chart"
                :disabled="Boolean(transformingType)"
                @click="runTransform('data_chart')"
              >
                <span class="transform-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"></line><line x1="12" y1="20" x2="12" y2="4"></line><line x1="6" y1="20" x2="6" y2="14"></line></svg>
                </span>
                <span class="transform-name">数据图表</span>
              </button>
            </div>
            <div class="transform-custom-pill">
              <input v-model="customPrompt" type="text" placeholder="自定义生成..." autocomplete="off" />
              <button :disabled="Boolean(transformingType)" @click="runCustomTransform">
                {{ transformingType === "custom" ? "生成中..." : "生成" }}
              </button>
            </div>
            <div
              id="infographStyleDropdown"
              class="style-dropdown"
              v-show="infographStyleDropdownVisible"
            >
              <div class="style-dropdown-header">
                <span>选择信息图风格</span>
                <button class="style-dropdown-close" @click="infographStyleDropdownVisible = false">×</button>
              </div>
              <div class="style-dropdown-list" id="infographStyleList">
                <div
                  v-for="style in infographStyleOptions"
                  :key="style.id || 'default'"
                  class="style-item"
                  :class="{ selected: (selectedInfographStyle || '') === style.id }"
                  @click="selectInfographStyle(style.id)"
                >
                  <div class="style-item-name">
                    <span v-if="(selectedInfographStyle || '') === style.id" class="check-icon">✓</span>
                    {{ style.name }}
                  </div>
                  <div class="style-item-desc">{{ style.description }}</div>
                </div>
              </div>
            </div>
            </div>
          </div>

          <div class="workspace-section section-notes">
            <div class="panel-header">
              <h2 class="panel-title">笔记 ({{ notes.length }})</h2>
              <button class="btn-text" @click="activeCenterTab = 'notes_list'">详情</button>
            </div>
            <p v-if="notesLoading" class="muted">笔记加载中...</p>
            <p v-else-if="notesError" class="error">笔记加载失败：{{ notesError }}</p>
            <ul v-else-if="activeNoteView === 'notes_list'" class="note-list">
            <li
              v-for="item in notes"
              :key="item.id"
              class="note-item"
              :class="{ active: selectedNote?.id === item.id }"
              @click="noteStore.selectNote(item.id)"
            >
              <h4 class="note-title">{{ item.title }}</h4>
              <p class="note-preview">{{ toPlainText(item.content) }}</p>
              <div class="note-meta">
                <span class="note-date">{{ formatDate(item.created_at) }}</span>
                <span class="note-sources">{{ item.source_ids?.length || 0 }} 来源</span>
              </div>
              <button class="btn-delete-note" title="删除笔记" @click.stop="deleteNote(item.id)">
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="3" y1="3" x2="11" y2="11"></line>
                  <line x1="11" y1="3" x2="3" y2="11"></line>
                </svg>
              </button>
            </li>
            </ul>
            <div v-else-if="selectedNote" class="note-detail">
              <h3>{{ selectedNote.title }}</h3>
              <div class="note-meta">{{ selectedNote.type }}</div>
              <div
                v-if="shouldRenderRichNote"
                ref="sidebarNoteContentRef"
                class="markdown-content"
                v-html="selectedNoteRenderedHtml"
              ></div>
              <div v-else-if="shouldRenderPPTSlides" class="ppt-slides-view">
                <div class="ppt-slides-toolbar">
                  <button type="button" class="btn-slide-nav" :disabled="pptPrevDisabled" @click="showPrevPPTSlide">
                    上一张
                  </button>
                  <span class="ppt-slides-indicator">{{ pptSlideIndicator }}</span>
                  <button type="button" class="btn-slide-nav" :disabled="pptNextDisabled" @click="showNextPPTSlide">
                    下一张
                  </button>
                </div>
                <div class="ppt-slide-image-wrap">
                  <img :src="currentPPTSlideUrl" :alt="`幻灯片 ${pptSlideIndicator}`" loading="lazy" />
                </div>
              </div>
              <div v-else-if="selectedNoteInfographImageUrl" class="markdown-content">
                <img :src="selectedNoteInfographImageUrl" alt="信息图" loading="lazy" />
              </div>
              <pre v-else>{{ selectedNote.content }}</pre>
            </div>
            <p v-else class="muted">暂无可展示笔记</p>
          </div>
        </aside>
      </main>

      <div class="modal-overlay" :class="{ active: addSourceModalVisible }" @click.self="closeAddSourceModal">
        <div class="modal" :class="{ active: addSourceModalVisible }">
          <div class="modal-header">
            <h3>添加来源</h3>
            <button class="btn-close" @click="closeAddSourceModal">×</button>
          </div>
          <div class="modal-body">
            <div class="source-tabs">
              <button class="source-tab" :class="{ active: addSourceTab === 'file' }" @click="addSourceTab = 'file'">
                上传文件
              </button>
              <button class="source-tab" :class="{ active: addSourceTab === 'text' }" @click="addSourceTab = 'text'">
                粘贴文本
              </button>
              <button class="source-tab" :class="{ active: addSourceTab === 'url' }" @click="addSourceTab = 'url'">
                网址
              </button>
            </div>

            <div class="source-content" :class="{ active: addSourceTab === 'file' }">
              <label class="drop-zone">
                <input type="file" hidden @change="onModalFileChange" />
                <p>{{ uploading ? "上传中..." : "点击选择文件上传" }}</p>
              </label>
            </div>

            <form class="source-content" :class="{ active: addSourceTab === 'text' }" @submit.prevent="submitTextSource">
              <div class="form-group">
                <label class="input-label">来源名称</label>
                <input v-model="textSourceName" class="input-field" type="text" placeholder="文本来源标题" required />
              </div>
              <div class="form-group">
                <label class="input-label">文本内容</label>
                <textarea
                  v-model="textSourceContent"
                  class="input-field"
                  rows="6"
                  placeholder="粘贴文本内容"
                  required
                ></textarea>
              </div>
              <div class="modal-actions">
                <button type="button" class="btn-secondary" @click="closeAddSourceModal">取消</button>
                <button type="submit" class="btn-primary" :disabled="uploading">添加文本</button>
              </div>
            </form>

            <form class="source-content" :class="{ active: addSourceTab === 'url' }" @submit.prevent="submitURLSource">
              <div class="form-group">
                <label class="input-label">网址</label>
                <input v-model="urlSourceValue" class="input-field" type="url" placeholder="https://example.com" required />
              </div>
              <div class="form-group">
                <label class="input-label">名称 (可选)</label>
                <input v-model="urlSourceName" class="input-field" type="text" placeholder="留空则使用网址" />
              </div>
              <div class="modal-actions">
                <button type="button" class="btn-secondary" @click="closeAddSourceModal">取消</button>
                <button type="submit" class="btn-primary" :disabled="uploading">添加网址</button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>
