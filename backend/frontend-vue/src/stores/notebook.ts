import { defineStore } from "pinia";
import { request } from "../api/request";

export interface Notebook {
  id: string;
  name: string;
  description?: string;
  created_at?: string;
  source_count?: number;
  note_count?: number;
  is_public?: boolean;
  public_token?: string;
}

/**
 * 笔记本状态管理
 */
export const useNotebookStore = defineStore("notebook", {
  state: () => ({
    notebooks: [] as Notebook[],
    loading: false,
    togglingShareId: "",
    deletingNotebookId: "",
    error: "",
  }),
  actions: {
    /**
     * 加载笔记本列表
     */
    async loadNotebooks() {
      this.loading = true;
      this.error = "";
      try {
        this.notebooks = await request<Notebook[]>("/api/notebooks/stats");
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
      } finally {
        this.loading = false;
      }
    },
    /**
     * 切换笔记本公开状态
     */
    async toggleNotebookPublic(notebookId: string, isPublic: boolean): Promise<Notebook> {
      this.togglingShareId = notebookId;
      this.error = "";
      try {
        const updated = await request<Notebook>(`/api/notebooks/${notebookId}/public`, {
          method: "PUT",
          body: JSON.stringify({ is_public: isPublic }),
        });
        this.notebooks = this.notebooks.map((item) => (item.id === updated.id ? updated : item));
        return updated;
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.togglingShareId = "";
      }
    },
    /**
     * 创建笔记本
     */
    async createNotebook(name: string, description = ""): Promise<Notebook> {
      const payloadName = name.trim();
      if (!payloadName) {
        throw new Error("笔记本名称不能为空");
      }
      const notebook = await request<Notebook>("/api/notebooks", {
        method: "POST",
        body: JSON.stringify({
          name: payloadName,
          description: description.trim() || undefined,
        }),
      });
      this.notebooks = [notebook, ...this.notebooks];
      return notebook;
    },
    /**
     * 删除笔记本
     */
    async deleteNotebook(notebookId: string) {
      this.deletingNotebookId = notebookId;
      this.error = "";
      try {
        await request<void>(`/api/notebooks/${notebookId}`, {
          method: "DELETE",
        });
        this.notebooks = this.notebooks.filter((item) => item.id !== notebookId);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.deletingNotebookId = "";
      }
    },
  },
});
