<template>
  <section class="split-3">
    <article class="card">
      <h3>服务状态</h3>
      <div class="row-1">
        <div>
          MySQL:
          <span :class="mysqlOK ? 'status-ok' : 'status-bad'">{{ mysqlText }}</span>
        </div>
        <div>
          ES:
          <span :class="esOK ? 'status-ok' : 'status-bad'">{{ esText }}</span>
        </div>
        <button class="primary" @click="console.checkHealth()">刷新健康状态</button>
      </div>
    </article>

    <article class="card">
      <h3>工作区概览</h3>
      <div class="row-1">
        <div>知识库数量: <strong>{{ s.kbs.length }}</strong></div>
        <div>当前知识库文档数: <strong>{{ s.docs.length }}</strong></div>
        <div>当前文档 Chunk 数: <strong>{{ s.chunks.length }}</strong></div>
        <button @click="console.refreshCurrentContext()">刷新上下文</button>
      </div>
    </article>

    <article class="card">
      <h3>快捷入口</h3>
      <div class="row-1">
        <RouterLink to="/knowledge"><button>管理知识库与文档</button></RouterLink>
        <RouterLink to="/search"><button>进入检索调试</button></RouterLink>
        <RouterLink to="/chat"><button>进入对话</button></RouterLink>
      </div>
    </article>
  </section>
</template>

<script setup>
import { computed } from 'vue';
import { RouterLink } from 'vue-router';
import { useRagConsole } from '../composables/useRagConsole';

const console = useRagConsole();
const s = console.state;

const mysqlText = computed(() => s.health?.services?.mysql || '未知');
const esText = computed(() => s.health?.services?.es || '未知');
const mysqlOK = computed(() => mysqlText.value === 'ok');
const esOK = computed(() => esText.value === 'ok');
</script>
