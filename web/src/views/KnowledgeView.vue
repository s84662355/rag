<template>
  <section class="split-2">
    <article class="card">
      <h3>知识库</h3>
      <div class="row">
        <input v-model="s.kbForm.name" placeholder="知识库名称" />
        <input v-model="s.kbForm.description" placeholder="知识库描述" />
      </div>
      <div class="row">
        <button class="primary" @click="console.createKB()">创建知识库</button>
        <button @click="console.loadKBs()">刷新知识库</button>
      </div>
      <div class="row">
        <select v-model.number="s.selectedKBID" @change="console.onKBChange()">
          <option :value="0">选择知识库</option>
          <option v-for="kb in s.kbs" :key="kb.id" :value="kb.id">
            {{ kb.id }} - {{ kb.name }}
          </option>
        </select>
        <button class="danger" @click="console.deleteKB()">删除当前知识库</button>
      </div>

      <h3 style="margin-top:12px;">导入文档</h3>
      <div class="row">
        <input type="file" ref="fileInput" />
        <button class="primary" @click="onUpload">上传 md/pdf/html/txt</button>
      </div>
      <div class="row">
        <input v-model="s.urlForm.url" placeholder="https://..." />
        <button @click="console.ingestURL()">抓取网页</button>
      </div>
      <div class="hint-box">文档按知识库隔离，请先选择知识库再导入。</div>
    </article>

    <article class="card">
      <h3>文档列表</h3>
      <div class="row">
        <button @click="console.loadDocs()">刷新文档</button>
        <button @click="goChunkPage">前往分片编辑</button>
      </div>

      <div class="list">
        <div
          v-for="doc in s.docs"
          :key="doc.id"
          class="item"
          :class="{ active: s.selectedDocID === doc.id }"
          @click="console.pickDoc(doc.id)">
          <div class="item-top">
            <strong>#{{ doc.id }} {{ doc.title }}</strong>
            <div class="item-actions">
              <button class="mini-btn" @click.stop="openChunks(doc.id)">查看分片</button>
              <button class="mini-btn warn" @click.stop="console.reindexDoc(doc.id)">重建索引</button>
              <button class="mini-btn danger" @click.stop="console.deleteDoc(doc.id)">删除</button>
            </div>
          </div>
          <div class="muted">{{ doc.source_type }} | {{ doc.status }} | {{ doc.source_uri }}</div>
        </div>
        <div v-if="!s.docs.length" class="item">
          <div class="muted">暂无文档，请先上传文件或抓取网页。</div>
        </div>
      </div>
    </article>
  </section>
</template>

<script setup>
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useRagConsole } from '../composables/useRagConsole';

const router = useRouter();
const console = useRagConsole();
const s = console.state;
const fileInput = ref(null);

async function onUpload() {
  await console.uploadFile(fileInput.value?.files?.[0]);
}

function goChunkPage() {
  router.push('/chunks');
}

async function openChunks(docID) {
  await console.pickDoc(docID);
  router.push('/chunks');
}
</script>
