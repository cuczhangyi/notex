import { defineStore } from "pinia";
import { request } from "../api/request";

interface SourceSummary {
  id: string;
  name: string;
  type: string;
}

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  created_at?: string;
  sources?: string[];
}

export interface ChatSession {
  id: string;
  notebook_id: string;
  title: string;
  summary?: string;
  messages: ChatMessage[];
  created_at?: string;
  updated_at?: string;
  metadata?: Record<string, unknown>;
}

interface ChatResponse {
  message: string;
  sources: SourceSummary[];
  session_id: string;
  message_id: string;
}

/**
 * 对话状态管理
 */
export const useChatStore = defineStore("chat", {
  state: () => ({
    sessions: [] as ChatSession[],
    activeSessionId: "",
    messages: [] as ChatMessage[],
    loadingSessions: false,
    loadingMessages: false,
    deletingSessionId: "",
    clearingSessions: false,
    sending: false,
    error: "",
  }),
  getters: {
    activeSession: (state) => state.sessions.find((item) => item.id === state.activeSessionId) || null,
  },
  actions: {
    /**
     * 加载会话列表
     */
    async loadSessions(notebookId: string) {
      this.loadingSessions = true;
      this.error = "";
      try {
        this.sessions = await request<ChatSession[]>(`/api/notebooks/${notebookId}/chat/sessions`);
        if (!this.activeSessionId && this.sessions.length > 0) {
          this.activeSessionId = this.sessions[0].id;
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
      } finally {
        this.loadingSessions = false;
      }
    },
    /**
     * 创建会话
     */
    async createSession(notebookId: string, title = "新会话") {
      this.error = "";
      try {
        const session = await request<ChatSession>(`/api/notebooks/${notebookId}/chat/sessions`, {
          method: "POST",
          body: JSON.stringify({ title }),
        });
        await this.loadSessions(notebookId);
        this.activeSessionId = session.id;
        this.messages = session.messages || [];
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      }
    },
    /**
     * 加载会话消息
     */
    async loadSessionDetail(notebookId: string, sessionId: string) {
      this.loadingMessages = true;
      this.error = "";
      try {
        const session = await request<ChatSession>(
          `/api/notebooks/${notebookId}/chat/sessions/${sessionId}`,
        );
        this.activeSessionId = session.id;
        this.messages = session.messages || [];
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
      } finally {
        this.loadingMessages = false;
      }
    },
    /**
     * 确保存在可用会话
     */
    async ensureSession(notebookId: string) {
      if (this.activeSessionId) {
        return;
      }
      if (this.sessions.length === 0) {
        await this.createSession(notebookId);
        return;
      }
      this.activeSessionId = this.sessions[0].id;
    },
    /**
     * 发送消息
     */
    async sendMessage(notebookId: string, text: string) {
      const message = text.trim();
      if (!message) {
        return;
      }
      this.sending = true;
      this.error = "";
      try {
        await this.ensureSession(notebookId);
        const sessionId = this.activeSessionId;
        if (!sessionId) {
          throw new Error("未找到可用会话");
        }

        this.messages.push({
          id: `local-user-${Date.now()}`,
          role: "user",
          content: message,
        });

        const response = await request<ChatResponse>(
          `/api/notebooks/${notebookId}/chat/sessions/${sessionId}/messages`,
          {
            method: "POST",
            body: JSON.stringify({ message }),
          },
        );

        this.messages.push({
          id: response.message_id || `local-assistant-${Date.now()}`,
          role: "assistant",
          content: response.message,
          sources: (response.sources || []).map((item) => item.id),
        });

        await this.loadSessions(notebookId);
      } catch (err) {
        const messageText = err instanceof Error ? err.message : String(err);
        this.error = messageText;
      } finally {
        this.sending = false;
      }
    },
    /**
     * 删除单个会话
     */
    async deleteSession(notebookId: string, sessionId: string) {
      this.deletingSessionId = sessionId;
      this.error = "";
      try {
        await request<void>(`/api/notebooks/${notebookId}/chat/sessions/${sessionId}`, {
          method: "DELETE",
        });
        const wasActive = this.activeSessionId === sessionId;
        this.sessions = this.sessions.filter((item) => item.id !== sessionId);
        if (!wasActive) {
          return;
        }
        if (this.sessions.length === 0) {
          this.activeSessionId = "";
          this.messages = [];
          return;
        }
        await this.loadSessionDetail(notebookId, this.sessions[0].id);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.deletingSessionId = "";
      }
    },
    /**
     * 清空会话历史（逐条删除）
     */
    async clearSessions(notebookId: string) {
      this.clearingSessions = true;
      this.error = "";
      try {
        const ids = this.sessions.map((item) => item.id);
        for (const id of ids) {
          await request<void>(`/api/notebooks/${notebookId}/chat/sessions/${id}`, {
            method: "DELETE",
          });
        }
        this.sessions = [];
        this.activeSessionId = "";
        this.messages = [];
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.clearingSessions = false;
      }
    },
    /**
     * 重置当前对话状态
     */
    reset() {
      this.sessions = [];
      this.activeSessionId = "";
      this.messages = [];
      this.error = "";
      this.loadingSessions = false;
      this.loadingMessages = false;
      this.deletingSessionId = "";
      this.clearingSessions = false;
      this.sending = false;
    },
  },
});
