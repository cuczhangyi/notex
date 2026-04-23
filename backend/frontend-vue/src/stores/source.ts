import { defineStore } from "pinia";
import { request } from "../api/request";

export interface SourceItem {
  id: string;
  notebook_id?: string;
  name: string;
  type: string;
  content?: string;
  file_name?: string;
  status?: string;
  file_size?: number;
  chunk_count?: number;
  progress?: number;
  error_msg?: string;
}

/**
 * 来源状态管理
 */
export const useSourceStore = defineStore("source", {
  state: () => ({
    items: [] as SourceItem[],
    loading: false,
    uploading: false,
    error: "",
    selectedSourceId: "",
  }),
  getters: {
    selectedItem: (state) => state.items.find((item) => item.id === state.selectedSourceId) || null,
  },
  actions: {
    /**
     * 选择来源
     */
    selectSource(id: string) {
      this.selectedSourceId = id;
    },
    /**
     * 加载来源列表
     */
    async loadSources(notebookId: string) {
      this.loading = true;
      this.error = "";
      try {
        this.items = await request<SourceItem[]>(`/api/notebooks/${notebookId}/sources`);
        if (this.items.length > 0 && !this.selectedSourceId) {
          this.selectedSourceId = this.items[0].id;
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
      } finally {
        this.loading = false;
      }
    },
    /**
     * 上传文件来源
     */
    async uploadFile(notebookId: string, file: File) {
      const formData = new FormData();
      formData.append("file", file);
      formData.append("notebook_id", notebookId);

      this.uploading = true;
      this.error = "";
      try {
        const created = await request<SourceItem>("/api/upload", {
          method: "POST",
          body: formData,
        });
        await this.loadSources(notebookId);
        if (created?.id) {
          this.selectedSourceId = created.id;
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
      } finally {
        this.uploading = false;
      }
    },
    /**
     * 添加文本来源
     */
    async createTextSource(notebookId: string, name: string, content: string) {
      const payloadName = name.trim();
      const payloadContent = content.trim();
      if (!payloadName) {
        throw new Error("来源名称不能为空");
      }
      if (!payloadContent) {
        throw new Error("来源内容不能为空");
      }
      this.uploading = true;
      this.error = "";
      try {
        const created = await request<SourceItem>(`/api/notebooks/${notebookId}/sources`, {
          method: "POST",
          body: JSON.stringify({
            name: payloadName,
            type: "text",
            content: payloadContent,
          }),
        });
        await this.loadSources(notebookId);
        if (created?.id) {
          this.selectedSourceId = created.id;
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.uploading = false;
      }
    },
    /**
     * 添加 URL 来源
     */
    async createURLSource(notebookId: string, url: string, name = "") {
      const payloadURL = url.trim();
      const payloadName = (name.trim() || payloadURL).trim();
      if (!payloadURL) {
        throw new Error("网址不能为空");
      }
      this.uploading = true;
      this.error = "";
      try {
        const created = await request<SourceItem>(`/api/notebooks/${notebookId}/sources`, {
          method: "POST",
          body: JSON.stringify({
            name: payloadName,
            type: "url",
            url: payloadURL,
          }),
        });
        await this.loadSources(notebookId);
        if (created?.id) {
          this.selectedSourceId = created.id;
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.uploading = false;
      }
    },
    /**
     * 轮询处理中来源状态
     */
    async refreshProcessingStatuses(notebookId: string) {
      const processing = this.items.filter(
        (item) => item.status === "pending" || item.status === "processing",
      );
      if (processing.length === 0) {
        return;
      }

      const updates = await Promise.all(
        processing.map(async (item) => {
          try {
            const latest = await request<SourceItem>(`/api/sources/${item.id}`);
            return latest;
          } catch {
            return item;
          }
        }),
      );

      const byID = new Map(updates.map((item) => [item.id, item]));
      this.items = this.items.map((item) => byID.get(item.id) || item);

      const stillProcessing = this.items.some(
        (item) => item.status === "pending" || item.status === "processing",
      );
      if (!stillProcessing) {
        await this.loadSources(notebookId);
      }
    },
  },
});
