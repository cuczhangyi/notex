import { defineStore } from "pinia";
import { request } from "../api/request";

export interface NoteItem {
  id: string;
  title: string;
  type: string;
  content: string;
  source_ids?: string[];
  metadata?: Record<string, unknown>;
  created_at?: string;
}

export interface TransformOptions {
  infographStyleId?: string | null;
}

export type TransformType =
  | "summary"
  | "faq"
  | "study_guide"
  | "outline"
  | "podcast"
  | "timeline"
  | "glossary"
  | "quiz"
  | "mindmap"
  | "infograph"
  | "ppt"
  | "insight"
  | "data_table"
  | "data_chart"
  | "custom";

/**
 * 笔记状态管理
 */
export const useNoteStore = defineStore("note", {
  state: () => ({
    items: [] as NoteItem[],
    loading: false,
    deletingNoteId: "",
    transformingType: "",
    error: "",
    selectedNoteId: "",
  }),
  getters: {
    selectedItem: (state) => state.items.find((item) => item.id === state.selectedNoteId) || null,
  },
  actions: {
    /**
     * 选择笔记
     */
    selectNote(id: string) {
      this.selectedNoteId = id;
    },
    /**
     * 加载笔记列表
     */
    async loadNotes(notebookId: string) {
      this.loading = true;
      this.error = "";
      try {
        this.items = await request<NoteItem[]>(`/api/notebooks/${notebookId}/notes`);
        if (this.items.length > 0 && !this.selectedNoteId) {
          this.selectedNoteId = this.items[0].id;
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
      } finally {
        this.loading = false;
      }
    },
    /**
     * 执行笔记转换并插入列表
     */
    async transformNote(
      notebookId: string,
      type: TransformType,
      sourceIds: string[],
      customPrompt?: string,
      options?: TransformOptions,
    ) {
      this.transformingType = type;
      this.error = "";
      try {
        const note = await request<NoteItem>(`/api/notebooks/${notebookId}/transform`, {
          method: "POST",
          body: JSON.stringify({
            type,
            prompt: customPrompt || undefined,
            source_ids: sourceIds,
            length: "medium",
            format: "markdown",
            style:
              type === "infograph" && options?.infographStyleId
                ? options.infographStyleId
                : undefined,
          }),
        });
        this.items = [note, ...this.items];
        this.selectedNoteId = note.id;
        return note;
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.transformingType = "";
      }
    },
    /**
     * 删除指定笔记并同步本地状态
     */
    async deleteNote(notebookId: string, noteId: string) {
      this.deletingNoteId = noteId;
      this.error = "";
      try {
        await request<void>(`/api/notebooks/${notebookId}/notes/${noteId}`, {
          method: "DELETE",
        });
        this.items = this.items.filter((item) => item.id !== noteId);
        if (this.selectedNoteId === noteId) {
          this.selectedNoteId = this.items[0]?.id || "";
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.error = message;
        throw err;
      } finally {
        this.deletingNoteId = "";
      }
    },
  },
});
