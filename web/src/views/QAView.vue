<template>
  <section class="split-2">
    <article class="card">
      <h3>生成 QA 对</h3>
      <div class="row">
        <input v-model.number="s.qaForm.doc_id" placeholder="文档 ID" />
        <input v-model.number="s.qaForm.limit" placeholder="分片数量上限" />
      </div>

      <div class="row">
        <button class="primary" @click="console.generateQA()">生成</button>
        <button @click="console.loadQA()">读取已有 QA</button>
      </div>

      <div class="hint-box">如果文档执行过重建索引，请重新生成 QA 对。</div>
    </article>

    <article class="card">
      <h3>QA 输出</h3>
      <pre>{{ s.qaResult || '暂无 QA 结果。' }}</pre>
    </article>
  </section>
</template>

<script setup>
import { useRagConsole } from '../composables/useRagConsole';

const console = useRagConsole();
const s = console.state;
</script>
