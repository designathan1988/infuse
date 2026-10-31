package core

// Message catalog (part 2).

func init() {
	RegisterMessages(map[string]map[string]string{
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
	})
}
