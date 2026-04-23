<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useNotebookStore } from "../stores/notebook";
import { useAuthStore } from "../stores/auth";
import { request } from "../api/request";
import type { Notebook } from "../stores/notebook";

interface PublicNotebook {
  id: string;
  name: string;
  description?: string;
  source_count?: number;
  note_count?: number;
  cover_image?: string;
  created_at?: string;
  public_token?: string;
}

const router = useRouter();
const notebookStore = useNotebookStore();
const authStore = useAuthStore();

const notebooks = computed(() => notebookStore.notebooks);
const loading = computed(() => notebookStore.loading);
const error = computed(() => notebookStore.error);
const togglingShareId = computed(() => notebookStore.togglingShareId);
const user = computed(() => authStore.user);
const isLoggedIn = computed(() => authStore.isLoggedIn);
const shareModalVisible = ref(false);
const shareLoading = ref(false);
const shareFeedback = ref("");
const shareError = ref("");
const currentShareNotebook = ref<Notebook | null>(null);
const publicLoading = ref(false);
const publicNotebooks = ref<PublicNotebook[]>([]);
const publicError = ref("");
const notebookModalVisible = ref(false);
const creatingNotebook = ref(false);
const notebookName = ref("");
const notebookDescription = ref("");
const loginModalVisible = ref(false);
const loginError = ref("");
const testModeEnabled = ref(false);
const testLoginLoading = ref(false);

const shareLink = computed(() => {
  if (!currentShareNotebook.value?.public_token) {
    return "";
  }
  return `${window.location.origin}/public/${currentShareNotebook.value.public_token}`;
});

/**
 * 打开指定笔记本
 */
function openNotebook(id: string) {
  router.push(`/notes/${id}`);
}

/**
 * 打开公开笔记本
 */
function openPublicNotebook(token?: string) {
  if (!token) {
    return;
  }
  router.push(`/public/${token}`);
}

/**
 * 打开分享弹层
 */
function openShareModal(item: Notebook) {
  currentShareNotebook.value = item;
  shareFeedback.value = "";
  shareError.value = "";
  shareModalVisible.value = true;
}

/**
 * 关闭分享弹层
 */
function closeShareModal() {
  shareModalVisible.value = false;
  currentShareNotebook.value = null;
  shareFeedback.value = "";
  shareError.value = "";
}

/**
 * 切换公开状态
 */
async function toggleShare() {
  const notebook = currentShareNotebook.value;
  if (!notebook) {
    return;
  }
  shareLoading.value = true;
  shareError.value = "";
  shareFeedback.value = "";
  try {
    const updated = await notebookStore.toggleNotebookPublic(notebook.id, !Boolean(notebook.is_public));
    currentShareNotebook.value = updated;
    shareFeedback.value = updated.is_public ? "笔记本已公开" : "笔记本已取消公开";
  } catch (err) {
    shareError.value = err instanceof Error ? err.message : String(err);
  } finally {
    shareLoading.value = false;
  }
}

/**
 * 复制分享链接
 */
async function copyShareLink() {
  if (!shareLink.value) {
    return;
  }
  shareError.value = "";
  try {
    await navigator.clipboard.writeText(shareLink.value);
    shareFeedback.value = "链接已复制到剪贴板";
  } catch {
    shareError.value = "复制失败，请手动复制";
  }
}

/**
 * 加载公开笔记本展示
 */
async function loadPublicNotebooks() {
  publicLoading.value = true;
  publicError.value = "";
  try {
    publicNotebooks.value = await request<PublicNotebook[]>("/public/notebooks", { withAuth: false });
  } catch (err) {
    publicError.value = err instanceof Error ? err.message : String(err);
  } finally {
    publicLoading.value = false;
  }
}

/**
 * 打开登录弹层
 */
async function openLoginModal() {
  loginModalVisible.value = true;
  loginError.value = "";
  await checkTestMode();
}

/**
 * 关闭登录弹层
 */
function closeLoginModal() {
  loginModalVisible.value = false;
}

/**
 * 检查是否开启测试登录模式
 */
async function checkTestMode() {
  try {
    const resp = await fetch("/auth/test-mode");
    if (!resp.ok) {
      testModeEnabled.value = false;
      return;
    }
    const data = (await resp.json()) as { enabled?: boolean };
    testModeEnabled.value = Boolean(data.enabled);
  } catch {
    testModeEnabled.value = false;
  }
}

/**
 * OAuth 登录
 */
function loginWithProvider(provider: "github" | "google") {
  closeLoginModal();
  const width = 600;
  const height = 700;
  const left = (window.screen.width - width) / 2;
  const top = (window.screen.height - height) / 2;
  window.open(
    `/auth/login/${provider}`,
    "NotexLogin",
    `width=${width},height=${height},top=${top},left=${left}`,
  );
  window.addEventListener(
    "message",
    async (event: MessageEvent) => {
      if (event.origin !== window.location.origin) {
        return;
      }
      const payload = event.data as { token?: string; user?: { id: string; name: string; avatar_url?: string } };
      if (!payload?.token) {
        return;
      }
      authStore.setToken(payload.token);
      if (payload.user) {
        authStore.setUser(payload.user);
      } else {
        await authStore.fetchMe();
      }
      await notebookStore.loadNotebooks();
    },
    { once: true },
  );
}

/**
 * 测试账号登录
 */
async function loginWithTestAccount() {
  closeLoginModal();
  testLoginLoading.value = true;
  loginError.value = "";
  try {
    const resp = await fetch("/auth/test-login", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
    });
    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || "测试登录失败");
    }
    const data = (await resp.json()) as {
      token: string;
      user?: { id: string; name: string; avatar_url?: string };
    };
    authStore.setToken(data.token);
    if (data.user) {
      authStore.setUser(data.user);
    } else {
      await authStore.fetchMe();
    }
    await notebookStore.loadNotebooks();
  } catch (err) {
    loginError.value = err instanceof Error ? err.message : String(err);
  } finally {
    testLoginLoading.value = false;
  }
}

/**
 * 退出登录
 */
function logout() {
  authStore.clearToken();
  void notebookStore.loadNotebooks();
}

/**
 * 创建笔记本并进入工作区
 */
function openNotebookModal() {
  notebookName.value = "";
  notebookDescription.value = "";
  notebookModalVisible.value = true;
}

/**
 * 关闭新建笔记本弹层
 */
function closeNotebookModal() {
  notebookModalVisible.value = false;
}

/**
 * 创建笔记本并进入工作区
 */
async function createNotebook() {
  const name = notebookName.value.trim();
  if (!name) {
    return;
  }
  creatingNotebook.value = true;
  try {
    const notebook = await notebookStore.createNotebook(name, notebookDescription.value);
    closeNotebookModal();
    await router.push(`/notes/${notebook.id}`);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    window.alert(`创建失败：${message}`);
  } finally {
    creatingNotebook.value = false;
  }
}

/**
 * 格式化卡片时间
 */
function formatCardDate(dateString?: string) {
  if (!dateString) {
    return "";
  }
  const date = new Date(dateString);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  if (diff < 60 * 1000) {
    return "刚刚";
  }
  if (diff < 60 * 60 * 1000) {
    return `${Math.floor(diff / (60 * 1000))}分钟前`;
  }
  if (diff < 24 * 60 * 60 * 1000) {
    return `${Math.floor(diff / (60 * 60 * 1000))}小时前`;
  }
  return date.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

onMounted(async () => {
  await Promise.all([notebookStore.loadNotebooks(), loadPublicNotebooks()]);
  if (authStore.token) {
    await authStore.fetchMe();
  }
});
</script>

<template>
  <main class="app-container">
    <header class="app-header">
      <div class="header-brand">
        <div class="logo-mark" aria-hidden="true">
          <svg viewBox="0 0 40 40" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="4" y="4" width="32" height="32" rx="2" />
            <line x1="12" y1="12" x2="28" y2="12" />
            <line x1="12" y1="18" x2="24" y2="18" />
            <line x1="12" y1="24" x2="20" y2="24" />
          </svg>
        </div>
        <div class="logo-type">
          <span class="logo-name">NOTEX</span>
          <span class="logo-tagline">知识存档</span>
        </div>
      </div>
      <div class="header-actions">
        <a href="https://github.com/smallnest/notex" target="_blank" class="btn-github" title="GitHub 仓库">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
            <path
              d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.042-1.416-4.042-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.44-1.304.806-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"
            />
          </svg>
        </a>
        <div class="auth-container">
          <button v-if="!isLoggedIn" class="btn-primary btn-sm" @click="openLoginModal">登录</button>
          <div v-else class="user-profile">
            <img v-if="user?.avatar_url" :src="user.avatar_url" alt="Avatar" class="user-avatar" />
            <span class="user-name">{{ user?.name || "已登录用户" }}</span>
            <button class="btn-text btn-sm" @click="logout">退出</button>
          </div>
        </div>
      </div>
    </header>

    <section class="landing-page">
      <div class="landing-header">
        <div class="landing-brand">
          <h1>我的笔记本</h1>
          <p>管理和探索你的知识存档</p>
        </div>
        <button class="btn-primary btn-large" @click="openNotebookModal">
          <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="8" y1="2" x2="8" y2="14" />
            <line x1="2" y1="8" x2="14" y2="8" />
          </svg>
          新建笔记本
        </button>
      </div>

      <div class="public-showcase">
        <div class="public-showcase-header">
          <h2 class="public-showcase-title">精选公开笔记本</h2>
          <p class="public-showcase-subtitle">探索社区分享的知识</p>
        </div>
        <p v-if="publicLoading" class="muted">公开笔记本加载中...</p>
        <p v-else-if="publicError" class="error">公开笔记本加载失败：{{ publicError }}</p>
        <div v-else class="public-showcase-scroll">
          <div class="public-showcase-grid">
            <article
              v-for="item in publicNotebooks"
              :key="item.id"
              class="public-showcase-card"
              :class="{ 'has-cover-image': Boolean(item.cover_image) }"
              @click="openPublicNotebook(item.public_token)"
            >
              <div class="public-showcase-card-header">
                <div class="public-showcase-card-title">{{ item.name }}</div>
              </div>
              <p class="public-showcase-card-desc">{{ item.description || "公开笔记本" }}</p>
              <div class="public-showcase-card-footer">
                <div class="public-showcase-card-stats">
                  {{ item.source_count || 0 }} 来源 · {{ item.note_count || 0 }} 笔记
                </div>
              </div>
            </article>
          </div>
        </div>
      </div>

      <p v-if="loading" class="muted">笔记本加载中...</p>
      <p v-else-if="error" class="error">加载失败：{{ error }}</p>

      <div v-else class="notebook-grid">
        <article v-for="item in notebooks" :key="item.id" class="notebook-card" @click="openNotebook(item.id)">
          <div class="notebook-card-content">
            <div class="notebook-card-icon">
              <svg viewBox="0 0 40 40" fill="none" stroke="currentColor" stroke-width="1.5">
                <rect x="8" y="8" width="24" height="24" rx="2" />
                <line x1="14" y1="16" x2="26" y2="16" />
                <line x1="14" y1="22" x2="22" y2="22" />
              </svg>
            </div>
            <h3 class="notebook-card-name">{{ item.name }}</h3>
            <p class="notebook-card-desc">{{ item.description || "暂无描述" }}</p>
            <div class="notebook-card-stats">
              <span class="stat-sources">{{ item.source_count || 0 }} 来源</span>
              <span class="stat-notes">{{ item.note_count || 0 }} 笔记</span>
              <span class="stat-date">{{ formatCardDate(item.created_at) }}</span>
              <button
                class="btn-share-card"
                :class="{ active: Boolean(item.is_public) }"
                :disabled="togglingShareId === item.id"
                :title="item.is_public ? '已公开' : '分享'"
                @click.stop="openShareModal(item)"
              >
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5">
                  <circle cx="10" cy="4" r="2" />
                  <circle cx="4" cy="10" r="2" />
                  <circle cx="10" cy="10" r="2" />
                  <line x1="5.5" y1="9" x2="8.5" y2="5" />
                  <line x1="5.5" y1="11" x2="8.5" y2="11" />
                </svg>
              </button>
            </div>
          </div>
        </article>
      </div>
    </section>

    <div v-if="shareModalVisible" class="share-overlay" @click.self="closeShareModal">
      <section class="share-modal">
        <h3>笔记本分享</h3>
        <p class="muted">当前笔记本：{{ currentShareNotebook?.name }}</p>
        <p v-if="currentShareNotebook?.is_public" class="share-status public">笔记本已公开</p>
        <p v-else class="share-status private">笔记本未公开</p>

        <div v-if="currentShareNotebook?.is_public" class="share-link-row">
          <input class="share-link-input" type="text" readonly :value="shareLink" />
          <button class="btn" @click="copyShareLink">复制</button>
        </div>

        <p v-if="shareFeedback" class="share-feedback">{{ shareFeedback }}</p>
        <p v-if="shareError" class="error">{{ shareError }}</p>

        <div class="share-actions">
          <button class="btn" :disabled="shareLoading" @click="toggleShare">
            {{ shareLoading ? "处理中..." : currentShareNotebook?.is_public ? "取消公开" : "公开笔记本" }}
          </button>
          <button class="btn" @click="closeShareModal">关闭</button>
        </div>
      </section>
    </div>

    <div class="modal-overlay" :class="{ active: notebookModalVisible }" @click.self="closeNotebookModal">
      <div class="modal" :class="{ active: notebookModalVisible }">
        <div class="modal-header">
          <h3>新建笔记本</h3>
          <button class="btn-close" @click="closeNotebookModal">×</button>
        </div>
        <form class="modal-body" @submit.prevent="createNotebook">
          <div class="form-group">
            <label class="input-label">名称</label>
            <input
              v-model="notebookName"
              type="text"
              class="input-field"
              name="name"
              placeholder="研究笔记"
              required
              autofocus
            />
          </div>
          <div class="form-group">
            <label class="input-label">描述 (可选)</label>
            <textarea
              v-model="notebookDescription"
              class="input-field"
              name="description"
              placeholder="简要描述..."
              rows="2"
            />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="closeNotebookModal">取消</button>
            <button type="submit" class="btn-primary" :disabled="creatingNotebook">
              {{ creatingNotebook ? "创建中..." : "创建笔记本" }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <div class="login-modal" :class="{ active: loginModalVisible }" @click.self="closeLoginModal">
      <div class="login-modal-content">
        <div class="login-modal-header">
          <h3>选择登录方式</h3>
          <button class="btn-close-login" @click="closeLoginModal">×</button>
        </div>
        <div class="login-modal-body">
          <button class="btn-login-provider" @click="loginWithProvider('github')">
            <svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
              <path
                d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"
              />
            </svg>
            使用 GitHub 登录
          </button>
          <button class="btn-login-provider" @click="loginWithProvider('google')">
            <svg width="20" height="20" viewBox="0 0 16 16">
              <path
                fill="#4285F4"
                d="M14.9 8.16c0-.95-.08-1.65-.21-2.37H8v4.4h3.83c-.17.96-.69 2.05-1.55 2.68v2.19h2.48c1.46-1.34 2.3-3.31 2.3-5.64z"
              />
              <path
                fill="#34A853"
                d="M8 16c2.07 0 3.83-.69 5.11-1.87l-2.48-2.19c-.69.46-1.57.73-2.63.73-2.02 0-3.74-1.37-4.35-3.19H1.11v2.26C2.38 13.89 4.99 16 8 16z"
              />
              <path
                fill="#FBBC05"
                d="M3.65 9.52c-.16-.46-.25-.95-.25-1.47s.09-1.01.25-1.47V4.48H1.11C.4 5.87 0 7.39 0 8s.4 2.13 1.11 3.52l2.54-2z"
              />
              <path
                fill="#EA4335"
                d="M8 3.24c1.14 0 2.17.39 2.98 1.15l2.2-2.2C11.83.87 10.07 0 8 0 4.99 0 2.38 2.11 1.11 4.48l2.54 2.26c.61-1.82 2.33-3.5 4.35-3.5z"
              />
            </svg>
            使用 Google 登录
          </button>
          <button v-if="testModeEnabled" class="btn-login-provider btn-test-login" :disabled="testLoginLoading" @click="loginWithTestAccount">
            <svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
              <path
                d="M8 0C3.58 0 0 3.58 0 8s3.58 8 8 8 8-3.58 8-8-3.58-8-8-8zm1 12H7V7h2v5zm0-6H7V4h2v2z"
              />
            </svg>
            {{ testLoginLoading ? "登录中..." : "使用测试账号登录" }}
          </button>
          <p v-if="loginError" class="error">{{ loginError }}</p>
        </div>
      </div>
    </div>
  </main>
</template>
