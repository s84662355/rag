import { reactive } from 'vue';
import axios from 'axios';

const state = reactive({
  apiBase: '',
  logs: [],
  health: null,
  kbs: [],
  kbForm: { name: '', description: '' },
  selectedKBID: 0,
  docs: [],
  selectedDocID: 0,
  urlForm: { url: '' },
  chunks: [],
  editChunkID: 0,
  editContent: '',
  searchForm: {
    query: '',
    top_k: 5,
    use_rewrite: true,
    use_rerank: true
  },
  searchResult: '',
  qaForm: {
    doc_id: 0,
    limit: 5
  },
  qaResult: '',
  chatForm: {
    session_id: '',
    question: '',
    top_k: 5
  },
  chatResult: ''
});

function api(path) {
  return `${state.apiBase || ''}${path}`;
}

function log(msg) {
  const line = `[${new Date().toLocaleTimeString()}] ${msg}`;
  state.logs.unshift(line);
  if (state.logs.length > 200) state.logs.length = 200;
}

function errorText(err) {
  if (err?.response?.data?.error) {
    return err.response.data.error;
  }
  return err?.message || String(err);
}

function handleError(prefix, err) {
  const msg = `${prefix}: ${errorText(err)}`;
  log(msg);
  window.alert(msg);
}

async function checkHealth() {
  try {
    const { data } = await axios.get(api('/api/health'));
    state.health = data;
    log(`健康检查 => ${JSON.stringify(data.services || {})}`);
  } catch (err) {
    handleError('健康检查失败', err);
  }
}

async function loadKBs() {
  try {
    const { data } = await axios.get(api('/api/kbs'));
    state.kbs = data.items || [];
    log(`知识库已加载: ${state.kbs.length}`);
  } catch (err) {
    handleError('加载知识库失败', err);
  }
}

async function createKB() {
  if (!state.kbForm.name.trim()) {
    window.alert('请输入知识库名称');
    return;
  }
  try {
    await axios.post(api('/api/kbs'), state.kbForm);
    state.kbForm = { name: '', description: '' };
    await loadKBs();
    log('知识库创建成功');
  } catch (err) {
    handleError('创建知识库失败', err);
  }
}

async function deleteKB() {
  if (!state.selectedKBID) {
    window.alert('请先选择知识库');
    return;
  }
  if (!window.confirm('确认删除当前知识库及其所有文档吗？')) return;

  try {
    await axios.delete(api(`/api/kbs/${state.selectedKBID}`));
    state.selectedKBID = 0;
    state.selectedDocID = 0;
    state.docs = [];
    state.chunks = [];
    await loadKBs();
    log('知识库已删除');
  } catch (err) {
    handleError('删除知识库失败', err);
  }
}

async function loadDocs() {
  if (!state.selectedKBID) {
    state.docs = [];
    return;
  }
  try {
    const { data } = await axios.get(api('/api/documents'), {
      params: { kb_id: state.selectedKBID }
    });
    state.docs = data.items || [];
    log(`文档已加载: ${state.docs.length}`);
  } catch (err) {
    handleError('加载文档失败', err);
  }
}

async function onKBChange() {
  state.selectedDocID = 0;
  state.chunks = [];
  await loadDocs();
}

async function uploadFile(file) {
  if (!state.selectedKBID) {
    window.alert('请先选择知识库');
    return;
  }
  if (!file) {
    window.alert('请选择文件');
    return;
  }

  try {
    const fd = new FormData();
    fd.append('kb_id', String(state.selectedKBID));
    fd.append('file', file);
    const { data } = await axios.post(api('/api/documents/upload'), fd, {
      headers: { 'Content-Type': 'multipart/form-data' }
    });
    log(`上传成功，chunk=${data.chunk_count}`);
    await loadDocs();
  } catch (err) {
    handleError('上传失败', err);
  }
}

async function ingestURL() {
  if (!state.selectedKBID) {
    window.alert('请先选择知识库');
    return;
  }
  if (!state.urlForm.url.trim()) {
    window.alert('请输入 URL');
    return;
  }

  try {
    const payload = {
      kb_id: state.selectedKBID,
      url: state.urlForm.url.trim()
    };
    const { data } = await axios.post(api('/api/documents/url'), payload);
    log(`网页导入成功，chunk=${data.chunk_count}`);
    await loadDocs();
  } catch (err) {
    handleError('网页导入失败', err);
  }
}

async function deleteDoc(docID) {
  if (!docID) return;
  if (!window.confirm(`确认删除文档 #${docID} 吗？`)) return;

  try {
    await axios.delete(api(`/api/documents/${docID}`));
    if (state.selectedDocID === docID) {
      state.selectedDocID = 0;
      state.chunks = [];
    }
    await loadDocs();
    log(`文档已删除: #${docID}`);
  } catch (err) {
    handleError('删除文档失败', err);
  }
}

async function reindexDoc(docID) {
  if (!docID) return;
  if (!window.confirm(`确认重建文档 #${docID} 索引吗？`)) return;

  try {
    const { data } = await axios.post(api(`/api/documents/${docID}/reindex`));
    log(`重建索引完成: doc=${data.document_id}, chunks=${data.chunk_count}`);
    await loadDocs();
    if (state.selectedDocID === docID) {
      await loadChunks();
    }
  } catch (err) {
    handleError('重建索引失败', err);
  }
}

async function pickDoc(docID) {
  state.selectedDocID = docID;
  state.qaForm.doc_id = docID;
  await loadChunks();
}

async function loadChunks() {
  if (!state.selectedDocID) {
    state.chunks = [];
    return;
  }
  try {
    const { data } = await axios.get(api('/api/chunks'), {
      params: { doc_id: state.selectedDocID }
    });
    state.chunks = data.items || [];
    log(`Chunk 已加载: ${state.chunks.length}`);
  } catch (err) {
    handleError('加载 Chunk 失败', err);
  }
}

function selectChunk(chunk) {
  state.editChunkID = chunk.id;
  state.editContent = chunk.content;
}

async function saveChunk() {
  if (!state.editChunkID) {
    window.alert('请先选择 Chunk');
    return;
  }
  if (!state.editContent.trim()) {
    window.alert('Chunk 内容不能为空');
    return;
  }

  try {
    await axios.put(api(`/api/chunks/${state.editChunkID}`), {
      content: state.editContent
    });
    log(`Chunk 更新成功: #${state.editChunkID}`);
    await loadChunks();
  } catch (err) {
    handleError('保存 Chunk 失败', err);
  }
}

async function search(debug) {
  if (!state.selectedKBID) {
    window.alert('请先选择知识库');
    return;
  }
  if (!state.searchForm.query.trim()) {
    window.alert('请输入问题');
    return;
  }

  try {
    const payload = {
      ...state.searchForm,
      kb_id: state.selectedKBID
    };
    const path = debug ? '/api/retrieve/debug' : '/api/search';
    const { data } = await axios.post(api(path), payload);
    state.searchResult = JSON.stringify(data, null, 2);
    log('检索完成');
  } catch (err) {
    handleError('检索失败', err);
  }
}

async function generateQA() {
  if (!state.qaForm.doc_id) {
    window.alert('请输入 doc_id');
    return;
  }
  try {
    const { data } = await axios.post(api('/api/qa/generate'), state.qaForm);
    state.qaResult = JSON.stringify(data, null, 2);
    log(`QA 生成完成: ${data.generated}`);
  } catch (err) {
    handleError('生成 QA 失败', err);
  }
}

async function loadQA() {
  if (!state.qaForm.doc_id) {
    window.alert('请输入 doc_id');
    return;
  }
  try {
    const { data } = await axios.get(api('/api/qa'), {
      params: { doc_id: state.qaForm.doc_id }
    });
    state.qaResult = JSON.stringify(data, null, 2);
    log('QA 已加载');
  } catch (err) {
    handleError('加载 QA 失败', err);
  }
}

async function chat() {
  if (!state.selectedKBID) {
    window.alert('请先选择知识库');
    return;
  }
  if (!state.chatForm.question.trim()) {
    window.alert('请输入问题');
    return;
  }

  try {
    const payload = {
      kb_id: state.selectedKBID,
      session_id: state.chatForm.session_id,
      question: state.chatForm.question,
      top_k: state.chatForm.top_k
    };
    const { data } = await axios.post(api('/api/chat'), payload);
    state.chatForm.session_id = data.session_id || state.chatForm.session_id;
    state.chatResult = JSON.stringify(data, null, 2);
    log(`对话完成, session=${data.session_id || '-'}`);
  } catch (err) {
    handleError('对话失败', err);
  }
}

async function bootstrap() {
  await loadKBs();
  await checkHealth();
}

async function refreshCurrentContext() {
  await checkHealth();
  await loadKBs();
  if (state.selectedKBID) await loadDocs();
  if (state.selectedDocID) await loadChunks();
}

function clearLogs() {
  state.logs = [];
}

let singleton = null;

export function useRagConsole() {
  if (!singleton) {
    singleton = {
      state,
      api,
      log,
      checkHealth,
      loadKBs,
      createKB,
      deleteKB,
      onKBChange,
      loadDocs,
      uploadFile,
      ingestURL,
      deleteDoc,
      reindexDoc,
      pickDoc,
      loadChunks,
      selectChunk,
      saveChunk,
      search,
      generateQA,
      loadQA,
      chat,
      bootstrap,
      refreshCurrentContext,
      clearLogs
    };
  }
  return singleton;
}
