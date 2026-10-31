# PROMPT — infuse Studio: o agente de código vive dentro do filtro

> Use este prompt para orientar a construção da próxima fase do infuse. Ele descreve o que a
> aplicação deve conter, como deve ser construída e como saberemos que está pronta.
> Leia SPEC.md antes. Os princípios de lá valem aqui e não se reabrem.

---

## 0. A tarefa em uma frase

Transformar o infuse em uma aplicação completa de desenvolvimento com agente de IA — **infuse Studio** —
que contém tudo o que o Claude Code desktop contém, de modo que não seja necessário abrir o Claude Code
nem qualquer outra plataforma, e em que **o agente nasce dentro do filtro**: cada leitura, escrita,
comando e declaração de "pronto" passa pelo coador por construção, não por interposição externa.

A aplicação continua sendo um único executável (`infuse.exe`), sem runtime, sem instalação.
O que existe hoje (shell interposto, mapa, regras, jornal, vícios, painel) permanece e passa a ser
a base sobre a qual o agente roda.

## 1. Requisito que decide tudo: o modelo

Um agente precisa de um modelo. O Claude Code desktop usa a assinatura do claude.ai; isso não é
reaproveitável por outra aplicação. O infuse Studio precisa, portanto, de **credencial própria**:

- Chave de API da Anthropic (obrigatório no primeiro corte), OpenAI, Google;
- OAuth quando o provedor oferecer para aplicações de terceiros;
- Modelos locais via endpoint compatível (Ollama, LM Studio, vLLM);
- Bedrock e Vertex para quem usa nuvem corporativa.

A tela de configuração pede a chave, testa, mostra custo estimado por sessão e por dia. Sem
credencial válida, a aplicação funciona como hoje (filtro + painel), apenas sem o agente.

## 2. O que o Claude Code desktop contém — e o infuse Studio também deve conter

Esta é a lista de paridade. Cada item é um requisito; nada aqui é opcional no produto final,
embora a entrega seja em fatias (seção 5).

### 2.1 Loop do agente
- Conversa com o modelo com streaming de texto e de raciocínio (quando o modelo expõe).
- Chamada de ferramentas pelo modelo, execução, retorno do resultado, continuação — em loop até o
  modelo encerrar o turno ou o usuário interromper (Esc / botão parar).
- Execução de ferramentas em paralelo quando independentes.
- Montagem do prompt de sistema: identidade, ferramentas, instruções do projeto, data, ambiente,
  estado do git, e **o conteúdo do filtro** (mapa, base, regras, lições, oráculo).
- Gerenciamento de contexto: contagem de tokens em tempo real, compactação automática quando se
  aproxima do limite, compactação manual, preservação do que importa (decisões, arquivos tocados,
  critérios de pronto) na compactação.
- Seleção de modelo por sessão e por tarefa (modelo principal, modelo rápido para subagentes),
  nível de raciocínio/esforço, limite de gasto.
- Modo plano: o agente lê e propõe sem escrever; sai do modo plano com aprovação.
- Persistência de sessão: histórico completo, retomar sessão, listar sessões por projeto, renomear,
  exportar.
- Interrupção a qualquer momento, com o estado consistente (ferramenta em execução é cancelada e
  registrada como cancelada).

### 2.2 Ferramentas do agente (equivalentes às do Claude Code)
- **Bash** — executar comando com timeout, saída em streaming, execução em segundo plano com
  acompanhamento, diretório de trabalho persistente dentro do projeto. *Passa pelo filtro por construção.*
- **Read** — arquivos de texto com numeração de linhas e intervalo; imagens (PNG/JPG) exibidas ao
  modelo; PDF por páginas; notebooks Jupyter por célula.
- **Write** — criar/sobrescrever arquivo. *Passa pelo filtro.*
- **Edit** — substituição exata de trecho (único ou todas as ocorrências), com verificação de que o
  arquivo foi lido antes. *Passa pelo filtro.*
- **MultiEdit** — várias edições atômicas no mesmo arquivo.
- **Glob** — buscar arquivos por padrão, ordenado por modificação.
- **Grep** — busca por regex com contexto, filtro por tipo, contagem, lista de arquivos.
- **LS** — listar diretório. *Volta infundido com o mapa.*
- **WebFetch** — baixar página e extrair; **WebSearch** — buscar na web (provedor configurável).
- **Task list / Todo** — lista de tarefas visível ao usuário, com estados.
- **Agent (subagentes)** — lançar agentes com contexto próprio, em paralelo, com tipos definidos
  pelo usuário (`.infuse/agents/*.md`), com ferramentas restritas por tipo, e **revisor independente**
  como tipo embutido (recebe só o diff e os critérios; nunca o contexto de quem escreveu).
- **NotebookEdit** — editar células de notebook.
- **AskUserQuestion** — perguntas de múltipla escolha ao usuário na interface.
- **Skill** — carregar uma skill (`SKILL.md`) sob demanda.
- **Cliente MCP** — conectar servidores MCP (stdio e HTTP), descobrir tools/resources/prompts,
  autenticação OAuth, habilitar/desabilitar por projeto, ver estado de conexão. O REA, por exemplo,
  entra por aqui.
- **ExitPlanMode / EnterPlanMode**.
- **Computer use / navegador** — item de paridade de fase tardia (seção 5); registrar como requisito.

### 2.3 Memória e instruções
- Hierarquia de instruções: global do usuário, do projeto, local não versionada, e por subpasta
  (carregada ao entrar na pasta). Compatível com `CLAUDE.md` e `AGENTS.md` existentes para quem migra.
- Memória persistente: o agente pode salvar notas (`#` ou comando) em arquivo de memória por projeto
  e global; o usuário edita no painel.
- **No infuse, instrução não é o mecanismo principal**: o mapa, a base, as regras e o oráculo são.
  Instruções existem por paridade e migração.

### 2.4 Extensibilidade
- Comandos de barra personalizados (`.infuse/commands/*.md`), com argumentos.
- Skills (`.infuse/skills/<nome>/SKILL.md`) e descoberta automática.
- Hooks por evento (antes/depois de ferramenta, ao enviar prompt, ao parar, ao iniciar sessão,
  antes de compactar, notificação) — como **redundância opcional**; o filtro não depende deles.
- Plugins e marketplace: instalar pacotes de comandos/skills/agentes/hooks/MCP de um repositório git.
- Estilos de saída (output styles) e tema.
- Configurações em camadas: administrada, usuário, projeto, local; variáveis de ambiente por projeto.
- Linha de status configurável.

### 2.5 Permissões e segurança
- Permissão por ferramenta e por padrão de argumento: perguntar, permitir, negar; listas de
  permitidos/negados; modo "aceitar edições"; modo plano; modo sem perguntas (bypass) com aviso.
- Sandbox para Bash (restringir rede e sistema de arquivos ao projeto quando o SO permitir).
- Proteção de caminhos: `.infuse/oracle/**` é somente leitura para o agente **no nível do executor
  de ferramentas**, não por regra de texto. Idem para arquivos marcados como protegidos pelo usuário.
- Confirmação para ações destrutivas (`rm -rf`, `git push --force`, reset, drop) com a transformação
  do filtro aplicada antes.

### 2.6 Git e revisão
- Estado do repositório na interface: branch, alterações, diff por arquivo com destaque.
- Commit, branch, worktree por tarefa (paralelismo isolado), PR via `gh`.
- **Checkpoints e voltar no tempo**: snapshot dos arquivos a cada turno; reverter a qualquer ponto
  da conversa (arquivos e/ou conversa).
- Revisão de código: pedir revisão do diff ao revisor independente, com resultado por arquivo/linha.

### 2.7 Interface (o que a janela do desktop tem)
- Janela própria (não só aba de navegador): WebView nativa do SO embutindo a interface, com o
  servidor local em 127.0.0.1. Vários projetos abertos em abas; várias sessões por projeto.
- Painel de conversa com streaming, blocos de ferramenta expansíveis (comando, saída, diff),
  rendering de markdown e código, imagens coladas (Ctrl+V) e arrastadas, menção de arquivo com `@`,
  histórico de prompts, atalhos de teclado, modo vim no editor de prompt.
- Árvore de arquivos do projeto com o mapa sobreposto (hotspots, dependentes).
- Visualizador de diff com aceitar/rejeitar por trecho.
- Terminal integrado (o shell interposto, visível).
- Lista de tarefas do agente ao lado da conversa.
- Notificações do sistema quando o agente precisa de você ou terminou.
- Tarefas agendadas: rodar um prompt em horário/intervalo, com histórico.
- Sessões remotas/continuar de outro dispositivo: item de paridade de fase tardia.
- Tudo que já existe no painel do infuse: regras por formulário, jornal, vícios, mapa, instalação.
- Idioma da interface: português e inglês.

### 2.8 Observabilidade e custo
- Uso de tokens e custo por turno, sessão, dia, projeto. Limite configurável com aviso e parada.
- Exportação de telemetria (OpenTelemetry) opcional.
- Log completo de cada turno (prompt enviado, resposta, ferramentas, tempos) consultável no painel.

## 3. O que o infuse Studio tem e o Claude Code não tem

Estes são os motivos de a aplicação existir. São os seis componentes de SPEC.md, agora nativos:

1. **Filtro por construção.** O executor de ferramentas do agente *é* o coador. Não há shim, não há
   interposição; Write/Edit/Bash/LS passam pela transformação e saem infundidos. O agente nunca vê o
   disco cru.
2. **Mapa vivo** na árvore, no prompt de sistema e em cada resultado de ferramenta.
3. **Bases** — ao criar projeto, escolhe-se a arquitetura de referência (API, web, CLI, biblioteca,
   app interativo com undo/ferramentas, jogo/3D, serviço com workers). A base é scaffold executável
   com modelo canônico, estados declarados, camadas e dependências permitidas, registros de extensão,
   tokens/config/tabelas num lugar só, persistência versionada, observabilidade, superfície observável.
4. **Oráculo** — "pronto" definido antes do código, na interface, em termos observáveis (o que se abre,
   clica, vê, mede), em várias dimensões; vira `.infuse/oracle/` somente leitura para o agente.
5. **Executor** — roda o oráculo num snapshot imutável, em navegador/processo/requisição, produz
   evidência (capturas, medições, saídas) e o veredito; o botão "Pronto?" da interface chama o
   executor, nunca o agente. "Não atingi, eis a evidência" é resultado válido e aparece como tal.
6. **Catálogo de vícios e lições** — detectores determinísticos com remédio, alimentados pelos 32 do
   catálogo-base e pelo que cada projeto ensina; estatísticas no painel.

E mais dois decorrentes:

7. **Revisor independente embutido** (seção 2.2) com contexto limpo.
8. **Relato honesto de impasse** como saída de primeira classe: botão do agente "não consigo avançar",
   que entrega hipóteses testadas, evidência e onde parou — sem penalidade na interface.

## 4. Como a própria aplicação deve ser construída

A aplicação é o primeiro projeto que obedece às suas próprias bases. Portanto:

- **Modelo canônico** do estado (projetos, sessões, turnos, ferramentas, permissões, custo) com um
  único dono por dado; interface e persistência leem dele, nunca mantêm cópias.
- **Máquina de estados** explícita para sessão (ociosa → pensando → executando ferramenta →
  aguardando permissão → aguardando usuário → compactando → encerrada) e para cada chamada de
  ferramenta. Cancelamento é transição, não exceção.
- **Camadas**: núcleo (agente, ferramentas, filtro) → serviços (sessões, git, MCP, executor) →
  interface (servidor local + WebView). Dependências só para dentro.
- **Registros**: ferramentas, provedores de modelo, bases, detectores de vício, adaptadores de
  observação do executor, comandos — adicionar um é registrar uma implementação, nunca tocar em
  condicionais.
- **Configuração e tokens** num esquema só; a interface não tem valores soltos.
- **Persistência versionada** desde o primeiro arquivo salvo (sessões, memória, oráculos), com migração.
- **Observabilidade**: todo evento do loop é um registro consultável; nada acontece sem rastro.
- **Superfície observável** para o oráculo da própria aplicação (seção 6).
- Um único binário Go; interface em HTML/JS embutida; WebView pela API nativa do SO
  (WebView2 no Windows, WKWebView no macOS, WebKitGTK no Linux), com fallback para o navegador.
- Sem dependências que exijam instalação pelo usuário.

## 5. Fatias de entrega (nesta ordem; cada uma utilizável sozinha)

1. **Agente mínimo dentro do filtro** — chave da Anthropic; loop com Bash, Read, Write, Edit, Glob,
   Grep, LS; permissões perguntar/permitir/negar; streaming; sessões persistentes; custo. Interface:
   conversa + árvore + diff. Já aqui, Write/Edit/Bash passam pelo filtro por construção e o
   `.infuse/oracle/` é somente leitura no executor de ferramentas.
2. **Paridade de ferramentas** — WebFetch/WebSearch, Task list, subagentes com revisor independente,
   AskUserQuestion, Skills, comandos de barra, instruções em camadas, memória, compactação.
3. **Git e tempo** — diff/commit/branch/worktree/PR, checkpoints e reverter.
4. **MCP** — cliente completo (stdio/HTTP, OAuth), gerenciamento por projeto.
5. **Oráculo e executor** — definição visual do "pronto", adaptadores de observação (navegador via
   motor embutido, processo, HTTP), evidência e veredito, botão "Pronto?".
6. **Bases** — catálogo inicial (API, web, CLI, app interativo), criação de projeto a partir de base,
   mapa pré-preenchido, detecção de "fora da base".
7. **Multi-provedor** — OpenAI, Google, locais, Bedrock/Vertex; modelo por tarefa.
8. **Janela nativa, abas, notificações, agendamento, hooks, plugins, temas, idiomas.**
9. **Tardio** — computer use/navegador do usuário, sessões remotas.

Cada fatia termina com o oráculo da própria fatia (seção 6) passando no executor, e com o relato de
evidência anexado à entrega. Nenhuma fatia é declarada pronta pelo agente que a construiu.

## 6. Pronto da própria aplicação (oráculo do infuse Studio)

A aplicação está pronta — em cada fatia, para o escopo dela — quando, sem nenhuma instrução dada ao
agente e com um usuário que nunca abre o Claude Code:

- Um pedido de feature num projeto resulta em código escrito **dentro da estrutura da base**, com o
  mapa atualizado, nenhum heredoc executado, cada escrita no jornal, e o diff visível com
  aceitar/rejeitar.
- O agente **não consegue** alterar nada em `.infuse/oracle/` nem em caminhos protegidos, por
  qualquer ferramenta, e a tentativa fica registrada.
- O botão "Pronto?" executa o oráculo num snapshot e devolve evidência; a declaração do agente não
  altera o veredito.
- Uma sessão interrompida no meio de uma ferramenta é retomada sem estado inconsistente.
- Reverter a um checkpoint restaura exatamente os arquivos daquele ponto.
- O custo exibido bate com o faturamento do provedor dentro de 1%.
- Toda ferramenta, provedor, base e detector é adicionado registrando uma implementação, sem editar
  condicionais em código existente (verificado por teste de arquitetura).
- Um revisor independente avalia cada diff antes de a fatia ser considerada entregue, e seu veredito
  é anexado.

## 7. O que fica explicitamente fora

- Qualquer mecanismo que dependa de texto dirigido ao comportamento do agente como garantia.
- Reaproveitar a assinatura ou os binários do Claude Code.
- Julgamento por LLM no caminho do filtro ou do executor.
- Declaração de pronto pelo próprio agente.

---

*Este prompt é um artefato do projeto. Mudanças nele passam pelo mesmo processo: editar, registrar a
decisão, e manter os princípios de SPEC.md intactos.*
