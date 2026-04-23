import { createRouter, createWebHistory } from "vue-router";
import LandingView from "../views/LandingView.vue";
import WorkspaceView from "../views/WorkspaceView.vue";
import PublicView from "../views/PublicView.vue";

/**
 * 应用路由定义
 * 保持与现有后端路由兼容：/, /notes/:id, /public/:token
 */
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", name: "landing", component: LandingView },
    { path: "/notes/:id", name: "workspace", component: WorkspaceView, meta: { requiresAuth: true } },
    { path: "/public/:token", name: "public", component: PublicView },
  ],
});

/**
 * 路由鉴权守卫：未登录用户访问工作区时跳转首页
 */
router.beforeEach((to) => {
  const token = localStorage.getItem("token");
  if (to.meta.requiresAuth && !token) {
    return { path: "/" };
  }
  return true;
});

export default router;
