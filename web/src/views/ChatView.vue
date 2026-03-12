<template>
  <section class="split-2">
    <article class="card">
      <h3>知识库对话</h3>
      <div class="row">
        <input v-model.number="s.selectedKBID" placeholder="知识库 ID" />
        <input v-model.number="s.chatForm.top_k" placeholder="召回数量 TopK" />
      </div>

      <div class="row">
        <input v-model="s.chatForm.session_id" placeholder="Session ID（可选）" />
        <button @click="s.chatForm.session_id = ''">新会话</button>
      </div>

      <div class="row">
        <input v-model="s.chatForm.question" placeholder="请输入问题..." />
        <button class="primary" @click="console.chat()">发送</button>
      </div>
    </article>

    <article class="card">
      <h3>对话结果</h3>
      <pre>{{ s.chatResult || '暂无返回结果。' }}</pre>
    </article>
  </section>
</template>

<script setup>
import { useRagConsole } from '../composables/useRagConsole';

const console = useRagConsole();
const s = console.state;
</script>
