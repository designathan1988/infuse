package core

// Message catalog (part 1).

func init() {
	RegisterMessages(map[string]map[string]string{
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
	})
}
