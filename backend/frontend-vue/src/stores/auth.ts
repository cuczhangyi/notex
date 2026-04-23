import { defineStore } from "pinia";
import { request } from "../api/request";

interface UserInfo {
  id: string;
  name: string;
  email?: string;
  avatar_url?: string;
}

interface MeResponse {
  user: UserInfo;
}

/**
 * 鉴权状态管理
 */
export const useAuthStore = defineStore("auth", {
  state: () => ({
    token: localStorage.getItem("token") || "",
    user: null as UserInfo | null,
    loading: false,
  }),
  getters: {
    isLoggedIn: (state) => Boolean(state.token),
  },
  actions: {
    /**
     * 设置当前登录用户信息
     */
    setUser(user: UserInfo | null) {
      this.user = user;
    },
    syncTokenToCookie() {
      if (!this.token) {
        return;
      }
      document.cookie = `token=${this.token}; path=/; SameSite=Lax`;
    },
    setToken(token: string) {
      this.token = token;
      localStorage.setItem("token", token);
      this.syncTokenToCookie();
    },
    clearToken() {
      this.token = "";
      this.user = null;
      localStorage.removeItem("token");
      document.cookie = "token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
    },
    async fetchMe() {
      if (!this.token) {
        return;
      }
      this.loading = true;
      try {
        const data = await request<MeResponse>("/api/auth/me");
        this.user = data.user;
      } finally {
        this.loading = false;
      }
    },
  },
});
