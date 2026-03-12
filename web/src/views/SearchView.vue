<template>
  <section class="split-2">
    <article class="card">
      <h3>检索调试</h3>
      <div class="row-3">
        <input v-model="s.searchForm.query" placeholder="输入问题" />
        <input v-model.number="s.searchForm.top_k" placeholder="召回数量 TopK" />
        <select v-model.number="s.selectedKBID" @change="console.onKBChange()">
          <option :value="0">知识库 ID</option>
          <option v-for="kb in s.kbs" :key="kb.id" :value="kb.id">{{ kb.id }}</option>
        </select>
      </div>

      <div class="row">
        <label><input type="checkbox" v-model="s.searchForm.use_rewrite" style="width:auto;"> 查询改写</label>
        <label><input type="checkbox" v-model="s.searchForm.use_rerank" style="width:auto;"> 结果重排</label>
      </div>

      <div class="row">
        <button class="primary" @click="console.search(false)">检索</button>
        <button @click="console.search(true)">检索 + 调试</button>
      </div>
    </article>

    <article class="card">
      <h3>结果</h3>
      <pre>{{ s.searchResult || '暂无结果。' }}</pre>
    </article>
  </section>
</template>

<script setup>
import { useRagConsole } from '../composables/useRagConsole';

const console = useRagConsole();
const s = console.state;
</script>
