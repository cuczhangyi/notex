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
    { path: "/notes/:id", name: "workspace", component: WorkspaceView },
    { path: "/public/:token", name: "public", component: PublicView },
  ],
});

export default router;
