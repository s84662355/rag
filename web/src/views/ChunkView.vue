<template>
  <section class="split-2">
    <article class="card">
      <h3>分片列表（Chunk）</h3>
      <div class="row">
        <input v-model.number="s.selectedDocID" placeholder="文档 ID" />
        <button @click="console.loadChunks()">加载分片</button>
      </div>

      <div class="list">
        <div v-for="c in s.chunks" :key="c.id" class="item" @click="console.selectChunk(c)">
          <div class="item-top">
            <strong>#{{ c.id }}</strong>
            <span class="muted">idx={{ c.chunk_index }} token~{{ c.token_count }}</span>
          </div>
          <div class="muted">{{ c.content.slice(0, 180) }}...</div>
        </div>

        <div v-if="!s.chunks.length" class="item">
          <div class="muted">暂无分片，请先选择文档。</div>
        </div>
      </div>
    </article>

    <article class="card">
      <h3>分片编辑器（Chunk）</h3>
      <div class="row">
        <input v-model.number="s.editChunkID" placeholder="分片 ID" />
        <button class="primary" @click="console.saveChunk()">保存并重新向量化</button>
      </div>
      <div class="row-1">
        <textarea v-model="s.editContent" placeholder="编辑分片内容"></textarea>
        <button @click="console.loadChunks()">刷新当前文档分片</button>
      </div>
    </article>
  </section>
</template>

<script setup>
import { useRagConsole } from '../composables/useRagConsole';

const console = useRagConsole();
const s = console.state;
</script>
