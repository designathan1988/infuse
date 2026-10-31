package main

// Messages of the tools: tool outputs and refusals, background processes, subagents and the review,
// the report and the contract, missions, memory, connectors, web search, skills, the agent's browser.
// English is what the model reads; pt-BR is what the user reads.

// SayErr is the tool output for an error: infuse's message recorded with the call, or the error's text.
func (tc *ToolContext) SayErr(err error) string {
	if m, ok := msgOf(err); ok {
		return tc.Say(m)
	}
	return err.Error()
}

// rootLang is the language the user reads infuse in, for code that has a project folder but no session.
func rootLang(root string) string {
	if l := EffectiveSettings(root).Language; l != "" {
		return l
	}
	return "pt-BR"
}

func init() {
	registerMessages(map[string]map[string]string{
		// ---------------------------------------------------------------- tools.go
		"tool.omitted": {
			"en":    "[... {count} characters omitted ...]",
			"pt-BR": "[... {count} caracteres omitidos ...]"},
		"tool.unknown_action": {
			"en":    "unknown action: {action}",
			"pt-BR": "ação desconhecida: {action}"},
		"path.outside": {
			"en":    "path outside the project: {path}",
			"pt-BR": "caminho fora do projeto: {path}"},
		"path.internal": {
			"en":    "{path} is an internal infuse file (sessions, journal, history), not part of the project. From .infuse/ the agent reads only the pinned documentation (.infuse/docs), the conventions, the map (.infuse/map/MAP.md) and its own captures",
			"pt-BR": "{path} é um arquivo interno do infuse (sessões, jornal, histórico), não do projeto. O agente lê do .infuse/ só a documentação fixada (.infuse/docs), as convenções, o mapa (.infuse/map/MAP.md) e as próprias capturas"},
		"path.protected": {
			"en":    "protected path, read-only for the agent: {path}. The oracle and the protected paths are set by the user and cannot be changed by the agent",
			"pt-BR": "caminho protegido, somente leitura para o agente: {path}. O oráculo e os caminhos protegidos são definidos pelo usuário e não podem ser alterados pelo agente"},
		"write.diagnostics": {
			"en":    "[diagnostics] problems found by the project's checker in this file:\n{problems}",
			"pt-BR": "[diagnóstico] problemas encontrados pelo verificador do projeto neste arquivo:\n{problems}"},
		"write.done": {
			"en":    "File written: {file} ({bytes} bytes)",
			"pt-BR": "Arquivo escrito: {file} ({bytes} bytes)"},
		"bash.empty": {
			"en":    "empty command",
			"pt-BR": "comando vazio"},
		"bash.not_started": {
			"en":    "did not start: {error}",
			"pt-BR": "não iniciou: {error}"},
		"bash.bg_running": {
			"en":    "process {id} running in the background (pid {pid}). Read the new output with BashOutput(id=\"{id}\"); stop it with KillShell. The user follows it in the Processes pane.",
			"pt-BR": "processo {id} em segundo plano (pid {pid}), rodando. Leia a saída nova com BashOutput(id=\"{id}\"); encerre com KillShell. O usuário acompanha no painel Processos."},
		"bash.bg_finished": {
			"en":    "process {id} in the background (pid {pid}) has already finished. Read the new output with BashOutput(id=\"{id}\"); stop it with KillShell. The user follows it in the Processes pane.",
			"pt-BR": "processo {id} em segundo plano (pid {pid}), já terminou. Leia a saída nova com BashOutput(id=\"{id}\"); encerre com KillShell. O usuário acompanha no painel Processos."},
		"bash.bg_first": {
			"en":    "First 2 s of output:",
			"pt-BR": "Primeiros 2 s de saída:"},
		"bash.timeout": {
			"en":    "[timed out after {seconds}s]",
			"pt-BR": "[tempo esgotado após {seconds}s]"},
		"read.image_too_large": {
			"en":    "image too large to send to the model ({bytes} bytes; maximum 5 MB)",
			"pt-BR": "imagem grande demais para enviar ao modelo ({bytes} bytes; máximo 5 MB)"},
		"read.image_attached": {
			"en":    "image {file} ({bytes} bytes) attached to this reply for you to see",
			"pt-BR": "imagem {file} ({bytes} bytes) anexada a esta resposta para você ver"},
		"read.binary": {
			"en":    "binary file ({bytes} bytes)",
			"pt-BR": "arquivo binário ({bytes} bytes)"},
		"read.continues": {
			"en":    "[continues: the file has {lines} lines; read on from here with Read offset={offset}]",
			"pt-BR": "[continua: o arquivo tem {lines} linhas; leia a partir daqui com Read offset={offset}]"},
		"edit.unread": {
			"en":    "read the file with Read before editing it",
			"pt-BR": "leia o arquivo com Read antes de editá-lo"},
		"edit.not_found": {
			"en":    "old_string not found in the file",
			"pt-BR": "old_string não encontrado no arquivo"},
		"edit.ambiguous": {
			"en":    "old_string occurs {count} times; make it unique or use replace_all",
			"pt-BR": "old_string ocorre {count} vezes; torne-o único ou use replace_all"},
		"edit.done": {
			"en":    "Edited: {file} ({count} replacement(s))",
			"pt-BR": "Editado: {file} ({count} substituição(ões))"},
		"multiedit.none": {
			"en":    "no edits",
			"pt-BR": "nenhuma edição"},
		"multiedit.not_found": {
			"en":    "edit {n}: old_string not found (nothing was written)",
			"pt-BR": "edição {n}: old_string não encontrado (nada foi gravado)"},
		"multiedit.ambiguous": {
			"en":    "edit {n}: old_string occurs {count} times; make it unique or use replace_all (nothing was written)",
			"pt-BR": "edição {n}: old_string ocorre {count} vezes; torne-o único ou use replace_all (nada foi gravado)"},
		"multiedit.done": {
			"en":    "Edited {file}: {edits} edits, {count} replacement(s)",
			"pt-BR": "Editado {file}: {edits} edições, {count} substituição(ões)"},
		"glob.none": {
			"en":    "no files found",
			"pt-BR": "nenhum arquivo encontrado"},
		"grep.bad_regex": {
			"en":    "invalid regex: {error}",
			"pt-BR": "regex inválida: {error}"},
		"grep.none": {
			"en":    "no matches",
			"pt-BR": "nenhuma ocorrência"},
		"fetch.refused": {
			"en":    "WebFetch refused: {why}. The agent only fetches public addresses.",
			"pt-BR": "WebFetch recusado: {why}. O agente só busca endereços públicos."},
		"fetch.part": {
			"en":    "part {part} of {total}",
			"pt-BR": "parte {part} de {total}"},
		"fetch.continues": {
			"en":    "[continues: WebFetch with offset={offset} — read to the end before acting]",
			"pt-BR": "[continua: WebFetch com offset={offset} — leia até o fim antes de agir]"},
		"plan.empty": {
			"en":    "the plan needs at least one task",
			"pt-BR": "o plano precisa de pelo menos uma tarefa"},
		"plan.no_proof": {
			"en":    "plan refused: every open task needs a proof (the command, the page or the file that will prove it works). Without proof: {tasks}",
			"pt-BR": "plano recusado: cada tarefa aberta precisa de proof (o comando, a página ou o arquivo que comprovará que funciona). Sem proof: {tasks}"},
		"plan.recorded": {
			"en":    "plan recorded ({count} tasks); the user sees each one checked off as you update it",
			"pt-BR": "plano registrado ({count} tarefas); o usuário vê cada uma marcada conforme você atualiza"},
		"ask.no_answer": {
			"en":    "the user did not answer",
			"pt-BR": "o usuário não respondeu"},
		"ask.answer": {
			"en":    "User's answer: {answer}",
			"pt-BR": "Resposta do usuário: {answer}"},

		// ---------------------------------------------------------------- procs.go
		"proc.discarded": {
			"en":    "[… {bytes} older bytes discarded …]",
			"pt-BR": "[… {bytes} bytes mais antigos descartados …]"},
		"proc.no_such": {
			"en":    "process {id} does not exist in this session",
			"pt-BR": "processo {id} não existe nesta sessão"},
		"proc.note_stopped": {
			"en":    "[infuse] the background process {id} ({command}) was stopped.",
			"pt-BR": "[infuse] o processo em segundo plano {id} ({command}) foi encerrado."},
		"proc.note_exited": {
			"en":    "[infuse] the background process {id} ({command}) finished on its own, exit code {code}. Read its output with BashOutput.",
			"pt-BR": "[infuse] o processo em segundo plano {id} ({command}) terminou sozinho, código {code}. Leia a saída com BashOutput."},
		"proc.not_found": {
			"en":    "process not found: {id}. Processes in this session: {list}",
			"pt-BR": "processo não encontrado: {id}. Processos desta sessão: {list}"},
		"proc.none": {
			"en":    "none",
			"pt-BR": "nenhum"},
		"proc.list": {
			"en":    "{ids}",
			"pt-BR": "{ids}"},
		"proc.bad_filter": {
			"en":    "invalid filter: {error}",
			"pt-BR": "filter inválido: {error}"},
		"proc.head_running": {
			"en":    "[{id} · running]",
			"pt-BR": "[{id} · rodando]"},
		"proc.head_exited": {
			"en":    "[{id} · exited (exit code {code})]",
			"pt-BR": "[{id} · terminou (código {code})]"},
		"proc.head_stopped": {
			"en":    "[{id} · stopped (exit code {code})]",
			"pt-BR": "[{id} · encerrado (código {code})]"},
		"proc.head_ended": {
			"en":    "[{id} · ended]",
			"pt-BR": "[{id} · terminou]"},
		"proc.no_output": {
			"en":    "(no new output)",
			"pt-BR": "(nenhuma saída nova)"},
		"proc.stopped": {
			"en":    "process {id} stopped",
			"pt-BR": "processo {id} encerrado"},

		// ---------------------------------------------------------------- agent.go: subagents, the review
		"agent.tool_unavailable": {
			"en":    "tool unavailable for this subagent: {tool}",
			"pt-BR": "ferramenta indisponível para este subagente: {tool}"},
		"agent.failed": {
			"en":    "subagent failed: {error}",
			"pt-BR": "subagente falhou: {error}"},
		"agent.stopped": {
			"en":    "subagent stopped: {error}",
			"pt-BR": "subagente interrompido: {error}"},
		"agent.step_limit": {
			"en":    "subagent reached the step limit",
			"pt-BR": "subagente atingiu o limite de passos"},
		"review.busy": {
			"en":    "wait for the agent to finish before reviewing",
			"pt-BR": "espere o agente terminar para revisar"},
		"review.title": {
			"en":    "Independent review",
			"pt-BR": "Revisão independente"},
		"review.header": {
			"en":    "[Independent review] Findings of a reviewer with a clean context, who read the diff, the plan and the criteria:",
			"pt-BR": "[Revisão independente] Achados de um revisor com contexto limpo, que leu o diff, o plano e os critérios:"},
		"review.ack": {
			"en":    "Review received.",
			"pt-BR": "Revisão recebida."},

		// ---------------------------------------------------------------- report.go: the report, the contract
		"report.head": {
			"en":    "Report checked by infuse:",
			"pt-BR": "Relatório conferido pelo infuse:"},
		"report.claim": {
			"en":    "- [{status}] {text} — {evidence}",
			"pt-BR": "- [{status}] {text} — {evidence}"},
		"report.st_confirmed": {
			"en":    "confirmed",
			"pt-BR": "confirmado"},
		"report.st_partial": {
			"en":    "partial",
			"pt-BR": "parcial"},
		"report.st_unconfirmed": {
			"en":    "not confirmed",
			"pt-BR": "não confirmado"},
		"report.st_no_evidence": {
			"en":    "no evidence",
			"pt-BR": "sem evidência"},
		"report.ev_no_command": {
			"en":    "no command given",
			"pt-BR": "nenhum comando indicado"},
		"report.ev_ran": {
			"en":    "run in this turn without error",
			"pt-BR": "executado neste turno sem erro"},
		"report.ev_failed": {
			"en":    "run in this turn, but it failed: {output}",
			"pt-BR": "executado neste turno, mas falhou: {output}"},
		"report.ev_not_run": {
			"en":    "this command was not run in this turn",
			"pt-BR": "nenhuma execução deste comando neste turno"},
		"report.ev_changed": {
			"en":    "changed in this turn",
			"pt-BR": "alterado neste turno"},
		"report.ev_removed": {
			"en":    "removed in this turn",
			"pt-BR": "removido neste turno"},
		"report.ev_unchanged": {
			"en":    "the file exists, but it was not changed in this turn",
			"pt-BR": "o arquivo existe, mas não foi alterado neste turno"},
		"report.ev_missing": {
			"en":    "the file does not exist",
			"pt-BR": "o arquivo não existe"},
		"report.ev_none": {
			"en":    "a claim with no verifiable evidence",
			"pt-BR": "afirmação sem evidência verificável"},
		"report.sources_head": {
			"en":    "Sources consulted in this turn:",
			"pt-BR": "Fontes consultadas neste turno:"},
		"report.source": {
			"en":    "- {source}",
			"pt-BR": "- {source}"},
		"report.src_search": {
			"en":    "search: {query}",
			"pt-BR": "pesquisa: {query}"},
		"report.src_page": {
			"en":    "{url}",
			"pt-BR": "{url}"},
		"report.src_doc": {
			"en":    "pinned documentation: {name}",
			"pt-BR": "documentação fixada: {name}"},
		"report.unconfirmed": {
			"en":    "{count} claim(s) not confirmed by execution: fix the work or the claim; the user sees this check.",
			"pt-BR": "{count} afirmação(ões) não confirmada(s) pela execução: corrija o trabalho ou a afirmação; o usuário vê esta conferência."},
		"contract.no_criteria": {
			"en":    "give at least one criterion",
			"pt-BR": "informe ao menos um critério"},
		"contract.question": {
			"en":    "Goal: {goal}\n\nI will consider it done when:\n{criteria}\n\nConfirm? (or write what to adjust)",
			"pt-BR": "Objetivo: {goal}\n\nVou considerar pronto quando:\n{criteria}\n\nConfirma? (ou escreva o que ajustar)"},
		"contract.confirm": {
			"en":    "Confirm",
			"pt-BR": "Confirmar"},
		"contract.changes": {
			"en":    "The user asked for changes to the criteria: {answer}\nRewrite them and call Contract again.",
			"pt-BR": "O usuário pediu ajustes nos critérios: {answer}\nReformule e chame Contract de novo."},
		"contract.confirmed": {
			"en":    "Criteria confirmed by the user and stored in the session memory. Work until they are met and report with Report, with evidence for each one.",
			"pt-BR": "Critérios confirmados pelo usuário e guardados na memória da sessão. Trabalhe até cumpri-los e relate com Report, com evidência de cada um."},

		// ---------------------------------------------------------------- mission.go
		"mission.no_goal": {
			"en":    "describe the mission's goal",
			"pt-BR": "descreva o objetivo da missão"},
		"mission.busy": {
			"en":    "the agent is busy",
			"pt-BR": "o agente está ocupado"},
		"mission.brief": {
			"en":    "[Mission] Work on your own until the goal below is met. At the end of each turn infuse runs the Done? criteria on a copy of the project; if they fail, you receive the reasons and continue. If you conclude that it is not possible to finish (missing access, information or a decision from the user), call the Impasse tool with what you tried and the evidence: that ends the mission and is a valid result.\n\nGoal: {goal}",
			"pt-BR": "[Missão] Trabalhe sozinho até o objetivo abaixo ser cumprido. Ao fim de cada turno o infuse roda os critérios do Pronto? numa cópia do projeto; se falharem, você recebe os motivos e continua. Se concluir que não é possível terminar (falta acesso, informação ou uma decisão do usuário), chame a ferramenta Impasse com o que tentou e as evidências: isso encerra a missão e é um resultado válido.\n\nObjetivo: {goal}"},
		"mission.next": {
			"en":    "[Mission — turn {turn} of {max} · US$ {spent} of US$ {budget}] {body}{auditor}",
			"pt-BR": "[Missão — turno {turn} de {max} · US$ {spent} de US$ {budget}] {body}{auditor}"},
		"mission.no_criteria": {
			"en":    "There are no executable criteria in Done?, so infuse has no way to confirm the end. If the goal is met, report it with Report and call Impasse explaining that criteria are missing to confirm it; otherwise, continue.",
			"pt-BR": "Não há critérios executáveis no Pronto?, então o infuse não tem como confirmar o fim. Se o objetivo está cumprido, relate com Report e chame Impasse explicando que faltam critérios para confirmar; senão, continue."},
		"mission.cannot_verify": {
			"en":    "The criteria could not run: {error}\nFix what prevents the verification or call Impasse.",
			"pt-BR": "Os critérios não puderam rodar: {error}\nCorrija o que impede a verificação ou chame Impasse."},
		"mission.rejected": {
			"en":    "Done? rejected the current work. Criteria that failed:\n{failing}\nFix it and continue. Do not change the criteria or the tests to make them pass.",
			"pt-BR": "O Pronto? reprovou o trabalho atual. Critérios que falharam:\n{failing}\nCorrija e continue. Não altere os critérios nem os testes para passar."},
		"mission.auditor": {
			"en":    "\n\nNote from the flow auditor (advice, not an order): {note}",
			"pt-BR": "\n\nObservação do auditor de fluxo (conselho, não ordem): {note}"},
		"mission.end_passed": {
			"en":    "the Done? criteria passed",
			"pt-BR": "os critérios do Pronto? passaram"},
		"mission.end_budget": {
			"en":    "budget reached: US$ {spent} of US$ {budget}",
			"pt-BR": "orçamento atingido: US$ {spent} de US$ {budget}"},
		"mission.end_turns": {
			"en":    "limit of {max} turns reached",
			"pt-BR": "limite de {max} turnos atingido"},
		"mission.end_stopped": {
			"en":    "stopped by the user",
			"pt-BR": "parada pelo usuário"},
		"mission.end_cannot_continue": {
			"en":    "could not continue: {error}",
			"pt-BR": "não foi possível continuar: {error}"},
		"impasse.no_mission": {
			"en":    "there is no mission running; explain the impasse to the user in your answer",
			"pt-BR": "não há missão em andamento; explique o impasse ao usuário na resposta"},
		"impasse.recorded": {
			"en":    "Mission ended at an impasse. The user sees the reason, what was tried and what is needed. End the turn with a short summary.",
			"pt-BR": "Missão encerrada em impasse. O usuário vê o motivo, o que foi tentado e o que é preciso. Termine o turno com um resumo curto."},
		"sentinel.test_edit": {
			"en":    "a change to a test file in a request to make the tests pass",
			"pt-BR": "uma alteração em arquivo de teste num pedido para fazer os testes passarem"},

		// ---------------------------------------------------------------- memory.go
		"memory.empty_text": {
			"en":    "empty text",
			"pt-BR": "texto vazio"},
		"memory.overflow": {
			"en":    "({count} lower-priority item(s) did not fit; list them with Memory action=list)",
			"pt-BR": "({count} item(ns) de menor prioridade não couberam; liste com Memory action=list)"},
		"memory.empty": {
			"en":    "memory is empty",
			"pt-BR": "memória vazia"},
		"memory.recorded": {
			"en":    "recorded: {id}",
			"pt-BR": "registrado: {id}"},
		"memory.not_found": {
			"en":    "item not found: {id}",
			"pt-BR": "item não encontrado: {id}"},
		"memory.removed": {
			"en":    "removed: {id}",
			"pt-BR": "removido: {id}"},

		// ---------------------------------------------------------------- mcp.go: connectors
		"mcp.no_command": {
			"en":    "server has neither a command nor a url",
			"pt-BR": "servidor sem command nem url"},
		"mcp.not_found": {
			"en":    "{command} not found on this computer",
			"pt-BR": "{command} não encontrado neste computador"},
		"mcp.start_failed": {
			"en":    "could not start the server: {error}",
			"pt-BR": "não foi possível iniciar o servidor: {error}"},
		"mcp.step_failed": {
			"en":    "{step}: {error}",
			"pt-BR": "{step}: {error}"},
		"mcp.exited": {
			"en":    "the server exited",
			"pt-BR": "o servidor terminou"},
		"mcp.exited_err": {
			"en":    "the server exited: {error}",
			"pt-BR": "o servidor terminou: {error}"},
		"mcp.stopped": {
			"en":    "server stopped",
			"pt-BR": "servidor parado"},
		"mcp.timeout": {
			"en":    "the server did not answer within 120 s",
			"pt-BR": "o servidor não respondeu em 120 s"},
		"mcp.stream_ended": {
			"en":    "the server's stream ended without an answer",
			"pt-BR": "o fluxo do servidor terminou sem resposta"},
		"mcp.call_failed": {
			"en":    "connector {name}: {error}",
			"pt-BR": "conector {name}: {error}"},
		"mcp.image_attached": {
			"en":    "[image attached to this reply]",
			"pt-BR": "[imagem anexada a esta resposta]"},

		// ---------------------------------------------------------------- websearch.go, skills.go
		"search.empty": {
			"en":    "empty search",
			"pt-BR": "pesquisa vazia"},
		"search.http": {
			"en":    "{engine} answered HTTP {status}",
			"pt-BR": "{engine} respondeu HTTP {status}"},
		"search.no_results": {
			"en":    "{engine} returned no results",
			"pt-BR": "{engine} não trouxe resultados"},
		"search.failed": {
			"en":    "search failed: {error}",
			"pt-BR": "pesquisa falhou: {error}"},
		"skill.head": {
			"en":    "Skill {name} (folder: {folder})",
			"pt-BR": "Skill {name} (pasta: {folder})"},
		"skill.not_found": {
			"en":    "skill not found: {name}",
			"pt-BR": "skill não encontrada: {name}"},

		// ---------------------------------------------------------------- fork.go, review.go (answers to the screen)
		"fork.no_message": {
			"en":    "no such message",
			"pt-BR": "mensagem inexistente"},
		"gh.missing": {
			"en":    "GitHub CLI (gh) not found: install it from https://cli.github.com and run `gh auth login`",
			"pt-BR": "GitHub CLI (gh) não encontrado: instale em https://cli.github.com e rode `gh auth login`"},
		"editor.not_found": {
			"en":    "editor not found",
			"pt-BR": "editor não encontrado"},
		"file.outside": {
			"en":    "outside the project",
			"pt-BR": "fora do projeto"},
		"file.changed_on_disk": {
			"en":    "the file changed on disk",
			"pt-BR": "o arquivo mudou no disco"},
		"ci.no_branch": {
			"en":    "there is no current branch",
			"pt-BR": "não há branch atual"},
		"ci.no_failure": {
			"en":    "no failed CI run on this branch",
			"pt-BR": "nenhuma execução de CI com falha neste branch"},

		// ---------------------------------------------------------------- oracle_suggest.go: criteria suggested for Done?
		"suggest.tests_pass": {
			"en":    "The tests pass",
			"pt-BR": "Os testes passam"},
		"suggest.builds": {
			"en":    "The project builds",
			"pt-BR": "O projeto compila"},
		"suggest.lint": {
			"en":    "No lint errors",
			"pt-BR": "Sem erros de lint"},
		"suggest.types": {
			"en":    "No type errors",
			"pt-BR": "Sem erros de tipo"},
		"suggest.home_opens": {
			"en":    "The home page opens",
			"pt-BR": "A página inicial abre"},
		"suggest.vet": {
			"en":    "No go vet problems",
			"pt-BR": "Sem problemas do go vet"},
		"suggest.python_valid": {
			"en":    "Every Python file is valid",
			"pt-BR": "Todo arquivo Python é válido"},
		"suggest.no_large_file": {
			"en":    "No file is too large",
			"pt-BR": "Nenhum arquivo grande demais"},
		"suggest.readme": {
			"en":    "The README exists",
			"pt-BR": "O README existe"},
		"suggest.why_script": {
			"en":    "Runs \"{command}\" (defined in package.json) and requires it to finish without error.",
			"pt-BR": "Roda \"{command}\" (definido em package.json) e exige que termine sem erro."},
		"suggest.why_run_ok": {
			"en":    "Runs \"{command}\" and requires it to finish without error.",
			"pt-BR": "Roda \"{command}\" e exige que termine sem erro."},
		"suggest.why_run": {
			"en":    "Runs \"{command}\".",
			"pt-BR": "Roda \"{command}\"."},
		"suggest.why_node_test": {
			"en":    "There are *.test.js files: runs Node's test runner and requires it to finish without error.",
			"pt-BR": "Há arquivos *.test.js: roda o executor de testes do Node e exige que termine sem erro."},
		"suggest.why_tsc": {
			"en":    "Checks the TypeScript types without emitting files (tsc --noEmit).",
			"pt-BR": "Confere os tipos do TypeScript sem gerar arquivos (tsc --noEmit)."},
		"suggest.why_page": {
			"en":    "Requests {url} and requires a 200 answer. The app must be running (Browser > Servers).",
			"pt-BR": "Pede {url} e exige resposta 200. O app precisa estar rodando (Navegador > Servidores)."},
		"suggest.why_vet": {
			"en":    "Runs \"go vet ./...\": points out common errors that compile.",
			"pt-BR": "Roda \"go vet ./...\": aponta erros comuns que compilam."},
		"suggest.why_compileall": {
			"en":    "Compiles each .py; reports syntax errors.",
			"pt-BR": "Compila cada .py; acusa erro de sintaxe."},
		"suggest.why_size": {
			"en":    "No *{ext} file goes over 400 lines: huge files point to mixed responsibilities.",
			"pt-BR": "Nenhum arquivo *{ext} passa de 400 linhas: arquivos enormes indicam responsabilidades misturadas."},
		"suggest.why_readme": {
			"en":    "Requires the file README.md. Change the expected text to check a section (e.g. \"Installation\").",
			"pt-BR": "Exige o arquivo README.md. Troque o texto esperado para conferir uma seção (ex.: \"Instalação\")."},

		// ---------------------------------------------------------------- browsertool.go, agentbrowser.go: the Browser tool
		"browser.give_selector": {
			"en":    "give a selector",
			"pt-BR": "informe selector"},
		"browser.give_url": {
			"en":    "give a url",
			"pt-BR": "informe url"},
		"browser.give_size": {
			"en":    "give width and height",
			"pt-BR": "informe width e height"},
		"browser.open_failed": {
			"en":    "did not open: {error}",
			"pt-BR": "não abriu: {error}"},
		"browser.open_failed_url": {
			"en":    "did not open {url}: {error}",
			"pt-BR": "não abriu {url}: {error}"},
		"browser.opened": {
			"en":    "opened: {where}",
			"pt-BR": "aberto: {where}"},
		"browser.went_back": {
			"en":    "back: {where}",
			"pt-BR": "voltou: {where}"},
		"browser.reloaded": {
			"en":    "reload: {where}",
			"pt-BR": "recarregou: {where}"},
		"browser.hover": {
			"en":    "cursor over <{tag}> \"{text}\"",
			"pt-BR": "cursor sobre <{tag}> \"{text}\""},
		"browser.disabled": {
			"en":    "<{tag}> \"{text}\" is disabled",
			"pt-BR": "<{tag}> \"{text}\" está desabilitado"},
		"browser.clicked": {
			"en":    "clicked <{tag}> \"{text}\" · now at {where}",
			"pt-BR": "clicou em <{tag}> \"{text}\" · agora em {where}"},
		"browser.typed": {
			"en":    "typed into {selector} · {where}",
			"pt-BR": "digitado em {selector} · {where}"},
		"browser.key": {
			"en":    "key {key} · {where}",
			"pt-BR": "tecla {key} · {where}"},
		"browser.no_key": {
			"en":    "no key in \"{combo}\"",
			"pt-BR": "tecla ausente em \"{combo}\""},
		"browser.scrolled_to": {
			"en":    "scrolled to <{tag}> \"{text}\"",
			"pt-BR": "rolou até <{tag}> \"{text}\""},
		"browser.scroll_pos": {
			"en":    "scroll position: {pos} of {max}",
			"pt-BR": "rolagem: {pos} de {max}"},
		"browser.dragged": {
			"en":    "dragged \"{from}\" to \"{to}\"",
			"pt-BR": "arrastou \"{from}\" para \"{to}\""},
		"browser.not_select": {
			"en":    "not a select",
			"pt-BR": "não é um select"},
		"browser.no_option": {
			"en":    "option not found: {options}",
			"pt-BR": "opção não encontrada: {options}"},
		"browser.selected": {
			"en":    "selected: {option}",
			"pt-BR": "selecionado: {option}"},
		"browser.upload_outside": {
			"en":    "the file must exist inside the project: {path}",
			"pt-BR": "o arquivo precisa existir dentro do projeto: {path}"},
		"browser.no_file_input": {
			"en":    "file input not found: {selector}",
			"pt-BR": "campo de arquivo não encontrado: {selector}"},
		"browser.uploaded": {
			"en":    "file attached to the input: {path}",
			"pt-BR": "arquivo enviado ao campo: {path}"},
		"browser.not_visible": {
			"en":    "element \"{selector}\" exists but is not visible",
			"pt-BR": "elemento \"{selector}\" existe mas não está visível"},
		"browser.not_found": {
			"en":    "element not found: {selector}",
			"pt-BR": "elemento não encontrado: {selector}"},
		"browser.waited": {
			"en":    "waited {ms} ms: {why}",
			"pt-BR": "esperou {ms} ms: {why}"},
		"browser.appeared": {
			"en":    "appeared after {ms} ms: <{tag}> \"{text}\"",
			"pt-BR": "apareceu em {ms} ms: <{tag}> \"{text}\""},
		"browser.script_error": {
			"en":    "script error: {error}",
			"pt-BR": "erro no script: {error}"},
		"browser.empty_capture": {
			"en":    "empty capture",
			"pt-BR": "captura vazia"},
		"browser.shot_screen": {
			"en":    "capture of the screen ({w}x{h} px) attached for you to see · {where} · .infuse/shots/{file}",
			"pt-BR": "captura da tela ({w}x{h} px) anexada para você ver · {where} · .infuse/shots/{file}"},
		"browser.shot_element": {
			"en":    "capture of the element {what} ({w}x{h} px) attached for you to see · {where} · .infuse/shots/{file}",
			"pt-BR": "captura de {what} ({w}x{h} px) anexada para você ver · {where} · .infuse/shots/{file}"},
		"browser.recording": {
			"en":    "recording: perform the actions, then call record_stop. You will receive the frames where the screen changed, with the timeline.",
			"pt-BR": "gravando: faça as ações e chame record_stop. Você receberá os quadros onde a tela mudou, com a linha do tempo."},
		"browser.no_frames": {
			"en":    "no frames captured: the screen did not change during the recording",
			"pt-BR": "nenhum quadro capturado: a tela não mudou durante a gravação"},
		"browser.rec_head": {
			"en":    "Recording of {seconds} s: {frames} frames captured, {keyframes} keyframes (where the screen changed), attached in order. Changes caused only by the agent's cursor do not count. Coordinates in page px ({w}x{h}). Files: {dir}",
			"pt-BR": "Gravação de {seconds} s: {frames} quadros capturados, {keyframes} quadros-chave (onde a tela mudou), anexados em ordem. Mudanças causadas só pelo cursor do agente não contam. Coordenadas em px da página ({w}x{h}). Arquivos: {dir}"},
		"browser.rec_event": {
			"en":    "t={t}s · {event}",
			"pt-BR": "t={t}s · {event}"},
		"browser.rec_frame_start": {
			"en":    "Image {img} · frame {frame} · t={t}s · start",
			"pt-BR": "Imagem {img} · quadro {frame} · t={t}s · início"},
		"browser.rec_frame_final": {
			"en":    "Image {img} · frame {frame} · t={t}s · final state (no change since the previous frame)",
			"pt-BR": "Imagem {img} · quadro {frame} · t={t}s · estado final (sem mudança desde o quadro anterior)"},
		"browser.rec_frame_changed": {
			"en":    "Image {img} · frame {frame} · t={t}s · {pct}% of the screen changed, in {areas} area(s): {where}",
			"pt-BR": "Imagem {img} · quadro {frame} · t={t}s · {pct}% da tela mudou, em {areas} área(s): {where}"},
		"browser.rec_crop": {
			"en":    "Image {img} · enlarged crop of area {area} of frame {frame}",
			"pt-BR": "Imagem {img} · recorte ampliado da área {area} do quadro {frame}"},
		"browser.ev_action": {
			"en":    "action: {action}",
			"pt-BR": "ação: {action}"},
		"browser.ev_exception": {
			"en":    "page exception: {text}",
			"pt-BR": "exceção na página: {text}"},
		"browser.ev_console": {
			"en":    "console.{type}: {text}",
			"pt-BR": "console.{type}: {text}"},
		"browser.ev_navigated": {
			"en":    "navigated to {url}",
			"pt-BR": "navegou para {url}"},
		"browser.ev_network": {
			"en":    "network: {method} {url} → {result}",
			"pt-BR": "rede: {method} {url} → {result}"},
		"browser.no_console": {
			"en":    "no console messages since the last check",
			"pt-BR": "nenhuma mensagem no console desde a última consulta"},
		"browser.no_requests": {
			"en":    "no requests recorded",
			"pt-BR": "nenhuma requisição registrada"},
		"browser.requests": {
			"en":    "{count} requests:",
			"pt-BR": "{count} requisições:"},
		"browser.timings": {
			"en":    "timings (ms): {values}",
			"pt-BR": "tempos (ms): {values}"},
		"browser.network_options": {
			"en":    "network: offline | 3g | 4g | none",
			"pt-BR": "rede: offline | 3g | 4g | none"},
		"browser.network_set": {
			"en":    "network: {network}",
			"pt-BR": "rede: {network}"},
		"browser.desktop": {
			"en":    "desktop {w}x{h} ({scale}x)",
			"pt-BR": "desktop {w}x{h} ({scale}x)"},
		"browser.device_options": {
			"en":    "device: {devices}",
			"pt-BR": "device: {devices}"},
		"browser.emulating": {
			"en":    "emulating {device}: {w}x{h} at density {density} (no touch)",
			"pt-BR": "emulando {device}: {w}x{h}, densidade {density}"},
		"browser.emulating_touch": {
			"en":    "emulating {device}: {w}x{h} at density {density} (touch)",
			"pt-BR": "emulando {device}: {w}x{h}, densidade {density}, toque"},
		"browser.viewport_warning": {
			"en":    "Warning: the page is laid out {width} px wide and scaled down to fit in {screen} px — <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"> is missing, or something is wider than the screen",
			"pt-BR": "Atenção: a página é desenhada com {width} px de largura e reduzida para caber em {screen} px — falta <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"> ou algo é mais largo que a tela"},
		"browser.viewport": {
			"en":    "screen of {w}x{h}",
			"pt-BR": "tela de {w}x{h}"},
		"browser.pdf": {
			"en":    "PDF saved: .infuse/shots/{file} ({kb} KB)",
			"pt-BR": "PDF salvo: .infuse/shots/{file} ({kb} KB)"},
	})
}
