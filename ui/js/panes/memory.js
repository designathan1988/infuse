'use strict';
// memory
// Structured memory pane.

// ---------------------------------------------------------------- Memory
// What the agent must always remember: typed items per session, project or every project. They go into
// every request (most important first) and survive compaction. The agent adds them with its Memory tool.
const MEM_TYPES = {
  correcao: ['Correção', 'Algo que você corrigiu e que não deve se repetir', 'use pnpm, não npm'],
  criterio: ['Critério de aceite', 'Condição combinada para o trabalho estar pronto', 'o formulário valida o e-mail antes de enviar'],
  decisao: ['Decisão', 'Uma escolha feita, e o porquê', 'CommonJS em vez de ESM: roda no Node 22 sem build'],
  beco: ['Beco sem saída', 'Algo já tentado que não funciona, e o motivo', 'vite 6 quebra o plugin de PWA; ficar no 5'],
  comando: ['Comando que funcionou', 'Como rodar, testar ou publicar', 'pnpm test -- --run'],
  arquivo: ['Arquivo', 'Um arquivo importante e o papel dele', 'src/auth.ts concentra a sessão do usuário'],
  fato: ['Fato', 'Qualquer informação útil sobre o projeto', 'a API de produção fica em api.exemplo.com'],
};
const MEM_SCOPES = { session: 'Esta sessão', project: 'Este projeto', global: 'Todos os projetos' };
const MEM_PRIO = ['', 'normal', 'importante', 'essencial'];
