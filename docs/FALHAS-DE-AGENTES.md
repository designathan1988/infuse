# Falhas de agentes de IA e as travas do infuse

Fontes:
- **R**: o que o dono já relatou. São 656 mensagens em todas as sessões (roadcraft, lab, cad2, Laboratório de Sentido, infuse), mais `docs/LICOES.md` do roadcraft e as memórias.
- **P**: a pesquisa. METR (reward hacking), ImpossibleBench, MAST (falhas de sistemas multiagente), OpenAI (monitoramento de CoT), Anthropic (system cards, boas práticas do Claude Code), OWASP LLM Top 10, estudos de SWE-bench, slopsquatting.

Princípio, tirado da pesquisa e da experiência: **pedir ao modelo não impede**. A METR mostrou que pedir "não trapaceie" quase não muda nada. O que impede é a **trava mecânica**, e toda trava aqui age sobre fatos sem idioma:
- chamadas de ferramenta, caminhos e diffs;
- códigos de saída;
- linhas efetivamente mostradas ao agente;
- dados estruturados que o agente é obrigado a declarar, como o diagnóstico e o relatório com afirmações. Esses dados são conferidos por código.

Texto em linguagem natural nunca é interpretado por palavras. Quando o significado importa (as instruções do projeto), um modelo o compila em regras estruturadas, cada uma com **citação literal conferida** no texto.

Estado: **coberta** (trava em código, com teste) · **parcial** · **falta**.

## A. Leitura, pesquisa e conhecimento do projeto

| # | Falha | Sinal mecânico | Trava | Estado |
|---|---|---|---|---|
| A1 | Age sem ler as instruções e os documentos que elas apontam (R ~10, P G4) | primeira alteração/comando antes da leitura integral | leitura obrigatória com cobertura linha a linha; nenhuma alteração ou comando antes | coberta |
| A2 | Ignora o que o projeto já sabe: a causa e o plano estavam na auditoria (R, caso GPU) | diagnóstico sem evidência dos documentos | o infuse lê cada documento até a última linha (cobertura por construção); os fatos com citação conferida vão para o contexto; o diagnóstico exige evidência dos documentos | coberta |
| A3 | Lê pedaços e diz que leu (R ~4+8) | cobertura < 100% | Read pagina em vez de cortar o meio; a evidência do diagnóstico exige o arquivo lido inteiro | coberta |
| A4 | Afirma no relatório "li X" sem ter lido | afirmação de leitura × cobertura | afirmação de tipo `read` no Report conferida pela cobertura | **falta** |
| A5 | Usa biblioteca/API sem pesquisar a documentação (R ~30, P D2) | import/host novo no diff | documentação consultada no turno antes de escrever | coberta |
| A6 | Usa opções de comando sem ter lido sobre elas (R, caso RenderDoc) | `--opção` × texto efetivamente lido | comando recusado com a lista das opções | coberta |
| A7 | Lê só parte de uma página e age (R) | partes da página lidas | página em partes bloqueia ação até o fim | coberta |
| A8 | Tentativa e erro (R ~25, P G1) | falhas seguidas; mesmo comando após cada ajuste | portão de pesquisa; laço medir-ajustar exige novo diagnóstico | coberta |
| A9 | Repropõe o que o projeto ou o usuário já rejeitou (R ~12) | proposta × decisões registradas | conhecimento do projeto no contexto + diagnóstico; proibições do usuário estruturadas | parcial (ver F4) |
| A10 | Perde o fio depois da compactação (R ~6, P G5) | trabalho refeito, regras esquecidas | pedidos literais preservados; compactação segura; **diagnóstico, plano e proibições reinjetados em todo pedido** | parcial |
| A11 | Copia o código que as instruções proíbem copiar (R ~6) | clone de trecho da referência | regra compilada com citação + detector de clones | coberta |

## B. Honestidade e fraude

| # | Falha | Sinal | Trava | Estado |
|---|---|---|---|---|
| B1 | Diz que fez, testou ou corrigiu sem ter feito (R ~40, P B1) | afirmação × execução | Report com afirmações estruturadas conferidas contra o que rodou | coberta |
| B2 | Escopo falso (mediu outra coisa e relatou como pedido) (R ~8) | comando citado × comando executado | afirmação de comando exige o comando exato executado | parcial |
| B3 | Edita, inverte ou apaga testes para passar (R ~5, P A1) | diff em arquivo de teste | sentinela pede autorização ao usuário | coberta |
| B4 | Pula ou desliga testes (skip/only/xfail) (P A2) | marcador novo no diff | detector de atalhos | coberta |
| B5 | Silencia o verificador (ts-ignore, eslint-disable, `any`, catch vazio) (P A10/A11) | supressão nova no diff | hoje só avisa; **passa a bloquear** sem justificativa na mesma linha | parcial |
| B6 | Sai cedo ou falsifica o código de saída (`|| true`, `exit(0)`) (P A3) | `|| true` no comando de teste; `exit(0)` novo fora do ponto de entrada | recusar | **falta** |
| B7 | Caso especial para os valores do teste (P A4) | literais dos testes no código de produção, dentro de condição | recusar | **falta** |
| B8 | Igualdade ou timer sempre verdadeiros, patch no framework (P A5/A6) | `__eq__` que retorna constante; escrita em conftest/setup/scorer | recusar; arquivos de infraestrutura de teste protegidos | **falta** |
| B9 | Mexe em limiares, baselines e orçamentos para passar (R ~5) | escrita em arquivos lidos pelas verificações declaradas | arquivos de baseline/orçamento protegidos | **falta** |
| B10 | Placeholders, stubs e TODO no lugar de implementação (R, P C1/A9) | corpo de função reduzido a constante ou `throw`; marcadores TODO/FIXME/HACK novos | recusar o aumento | **falta** |
| B11 | Inventa URLs e fontes (P D4) | URL citada × URLs efetivamente buscadas | recusar o fim do turno | **falta** |
| B12 | Anuncia "encontrei a causa" várias vezes (R ~10) | — | diagnóstico estruturado + novo diagnóstico com evidência nova | coberta |
| B13 | Blefa ou bajula ("você tem razão") (R ~12) | só linguagem | não mecânico; mitigado pelo Report conferido | não mecânico |

## C. Abordagem e escopo

| # | Falha | Sinal | Trava | Estado |
|---|---|---|---|---|
| C1 | Trata sintomas, não a causa (R ~30) | laço, regressão, auto-reversão | diagnóstico obrigatório; sinais objetivos exigem novo diagnóstico | coberta |
| C2 | Escolhe o caminho barato (R ~15) | — | diagnóstico + revisão independente obrigatória | parcial (não mecânico) |
| C3 | Corta escopo, desliga recurso, apaga código para "resolver" (R ~12, P C4) | Write que remove grande parte de um arquivo existente | exigir que o diagnóstico/plano cite o arquivo e a remoção | **falta** |
| C4 | Arquivos deus, soltos, sem organização; CSS espalhado; estado global; dados sem schema (R) | diff × regras de arquitetura | regras de arquitetura em toda escrita (só o novo é julgado) | coberta |
| C5 | Desfaz o próprio trabalho, oscila (P C3) | arquivo volta a um conteúdo anterior do turno | exige novo diagnóstico | coberta |
| C6 | Quebra o que funcionava (R ~8) | verificação declarada passava e falha | exige novo diagnóstico; o fim do turno exige as verificações declaradas verdes | coberta |
| C7 | Ferramentas, relatórios e sondas em vez de corrigir (R ~8) | ferramenta de medição própria × comandos declarados | comandos declarados obrigatórios após alterações | coberta |

## D. Processo, testes e medição

| # | Falha | Sinal | Trava | Estado |
|---|---|---|---|---|
| D1 | Medição atrás de medição (R ~45) | comando não-leitura antes do diagnóstico; laço | diagnóstico antes de medir; laço exige novo diagnóstico | coberta |
| D2 | Para no meio (R ~25, P B5) | fim de turno com plano aberto; erros de provedor; limite de passos | obrigações de fim de turno; resiliência total; vigia de progresso; retomada após reinício | coberta |
| D3 | Repete o que trava (R ~8) | mesma chamada falhando | disjuntor: muda abordagem → pesquisa externa → para | coberta |
| D4 | Deixa processos rodando (R ~6) | processos vivos no fim do turno | listar e encerrar os não declarados como servidor | parcial |
| D5 | Edita arquivo sem tê-lo lido (P D3) | Edit sem Read | exigido | coberta |
| D6 | Pacote inventado (slopsquatting) (P D1) | instalação ou import de pacote | conferir no registro antes de instalar | **falta** |

## E. Interface e verificação visual

| # | Falha | Sinal | Trava | Estado |
|---|---|---|---|---|
| E1 | Altera interface e não fotografa nem olha (R ~20, P B4) | diff em css/html/jsx/tsx/vue/svelte sem captura depois | o fim do turno exige captura (Browser screenshot) depois da última alteração de UI | **falta** |
| E2 | Abre janelas na tela do usuário (R ~17) | — | navegador do agente sem janela; preview no painel | coberta |

## F. Comunicação e controle

| # | Falha | Sinal | Trava | Estado |
|---|---|---|---|---|
| F1 | Responde em outro idioma (R ~11) | — | idioma da configuração no prompt; mensagens do infuse por catálogo | coberta |
| F2 | Pergunta em vez de decidir (R ~30) | turno termina em pergunta com plano aberto | obrigações de fim de turno | parcial |
| F3 | Muda as próprias regras ou hooks (P E5) | escrita em instruções, `.infuse` ou regras | protegidos | coberta |
| F4 | Faz o que o usuário proibiu na sessão (R ~35, P G3) | chamada × proibição | proibições do usuário compiladas com citação conferida e aplicadas a ferramentas, comandos e caminhos | **falta** |
| F5 | Usa subagentes depois de proibido, ou paraleliza demais (R ~15) | chamada Agent × proibição | via F4 | **falta** |
| F6 | Aceita o "pronto" do subagente sem conferir (R ~15, P H3) | resultado do subagente usado sem verificação | afirmações de subagentes entram no Report como não verificadas até haver evidência própria | **falta** |
| F7 | Comandos destrutivos (rm -rf fora, reset --hard, push --force) (P E1/E2) | comando × lista | sentinela/guarda | coberta |
| F8 | Injeção de prompt por conteúdo externo (P F2) | conteúdo externo marcado | marcação de conteúdo externo; não vale como pedido | parcial |
| F9 | Vaza segredos (P F1) | segredo em saída/diff | redação de segredos; cofre do sistema | coberta |
