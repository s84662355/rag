import { createRouter, createWebHistory } from 'vue-router';
import OverviewView from '../views/OverviewView.vue';
import KnowledgeView from '../views/KnowledgeView.vue';
import ChunkView from '../views/ChunkView.vue';
import SearchView from '../views/SearchView.vue';
import QAView from '../views/QAView.vue';
import ChatView from '../views/ChatView.vue';
import LogsView from '../views/LogsView.vue';

const routes = [
  { path: '/', redirect: '/overview' },
  { path: '/overview', component: OverviewView, meta: { title: '概览' } },
  { path: '/knowledge', component: KnowledgeView, meta: { title: '知识库与文档' } },
  { path: '/chunks', component: ChunkView, meta: { title: '分片编辑器' } },
  { path: '/search', component: SearchView, meta: { title: '检索调试' } },
  { path: '/qa', component: QAView, meta: { title: 'QA 生成' } },
  { path: '/chat', component: ChatView, meta: { title: '对话' } },
  { path: '/logs', component: LogsView, meta: { title: '日志' } }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

export default router;
