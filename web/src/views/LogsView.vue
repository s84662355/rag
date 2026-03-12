<template>
  <section class="card">
    <h3>运行日志</h3>
    <div class="row">
      <button @click="console.clearLogs()">清空日志</button>
      <button @click="console.refreshCurrentContext()">刷新上下文</button>
    </div>
    <pre>{{ logsText }}</pre>
  </section>
</template>

<script setup>
import { computed } from 'vue';
import { useRagConsole } from '../composables/useRagConsole';

const console = useRagConsole();
const s = console.state;

const logsText = computed(() => s.logs.length ? s.logs.join('\n') : '暂无日志。');
</script>
