<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <p class="brand-title">Go RAG</p>
        <p class="brand-sub">Vue 3 + Vite 管理后台</p>
      </div>

      <nav class="menu">
        <RouterLink
          v-for="item in menus"
          :key="item.to"
          :to="item.to"
          class="menu-btn"
          active-class="active"
        >
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="sidebar-meta">
        <div>当前知识库: <strong>{{ console.state.selectedKBID || '-' }}</strong></div>
        <div>当前文档: <strong>{{ console.state.selectedDocID || '-' }}</strong></div>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div>
          <h1 class="view-title">{{ currentTitle }}</h1>
          <p class="view-hint">
            文档解析 -> 切块 -> 向量化 -> 多路召回 -> 重排 -> QA/对话
            <span class="badge" v-if="console.state.selectedKBID">知识库 {{ console.state.selectedKBID }}</span>
            <span class="badge" v-if="console.state.selectedDocID">文档 {{ console.state.selectedDocID }}</span>
          </p>
        </div>

        <div class="toolbar">
          <input
            v-model="console.state.apiBase"
            placeholder="API 地址（默认同域）"
            style="width: 260px;"
          />
          <button @click="console.checkHealth()">健康检查</button>
        </div>
      </header>

      <RouterView />
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted } from 'vue';
import { RouterLink, RouterView, useRoute } from 'vue-router';
import { useRagConsole } from './composables/useRagConsole';

const route = useRoute();
const console = useRagConsole();

const menus = [
  { to: '/overview', label: '概览' },
  { to: '/knowledge', label: '知识库与文档' },
  { to: '/chunks', label: '分片编辑器' },
  { to: '/search', label: '检索调试' },
  { to: '/qa', label: 'QA 生成' },
  { to: '/chat', label: '对话' },
  { to: '/logs', label: '日志' }
];

const currentTitle = computed(() => route.meta.title || '控制台');

onMounted(async () => {
  await console.bootstrap();
});
</script>
