package main

// Catalog: diagnosis, project knowledge, resilience. English is what the model reads.

func init() {
	registerMessages(map[string]map[string]string{
		"diag.invalid": {
			"en":    "invalid diagnosis: {error}",
			"pt-BR": "diagnóstico inválido: {error}"},
		"diag.continue_refused": {
			"en":    "[infuse] Diagnosis refused: continues=true only carries the diagnosis of the immediately previous turn, when no new diagnosis was demanded. Register a full Diagnosis.",
			"pt-BR": "[infuse] Diagnóstico recusado: continues=true só leva adiante o diagnóstico do turno imediatamente anterior, quando nenhum novo foi exigido. Registre um diagnóstico completo."},
		"diag.continued": {
			"en":    "the previous diagnosis carries over to this turn (the user sees it marked as a continuation)",
			"pt-BR": "o diagnóstico anterior vale para este turno (o usuário o vê marcado como continuação)"},
		"diag.field_short": {
			"en":    "[infuse] Diagnosis refused: {field} is empty or too short to mean anything.",
			"pt-BR": "[infuse] Diagnóstico recusado: {field} está vazio ou curto demais para dizer algo."},
		"diag.need_two": {
			"en":    "[infuse] Diagnosis refused: at least 2 pieces of evidence (file and verbatim quote) are required.",
			"pt-BR": "[infuse] Diagnóstico recusado: são necessárias pelo menos 2 evidências (arquivo e trecho literal)."},
		"diag.refused": {
			"en":    "[infuse] Diagnosis refused:\n- {items}",
			"pt-BR": "[infuse] Diagnóstico recusado:\n- {items}"},
		"diag.ev_outside": {
			"en":    "evidence {n}: {file} is outside the project",
			"pt-BR": "evidência {n}: {file} está fora do projeto"},
		"diag.ev_missing": {
			"en":    "evidence {n}: {file} does not exist",
			"pt-BR": "evidência {n}: {file} não existe"},
		"diag.ev_quote": {
			"en":    "evidence {n}: the quote is not in {file}, word for word",
			"pt-BR": "evidência {n}: o trecho não está em {file}, palavra por palavra"},
		"diag.ev_unread": {
			"en":    "evidence {n}: you read {seen} of {total} lines of {file}; read it to the end first (Read offset={next})",
			"pt-BR": "evidência {n}: você leu {seen} de {total} linhas de {file}; leia até o fim antes (Read offset={next})"},
		"diag.no_knowledge": {
			"en":    "no evidence comes from the project's own documents ({docs}…). What the project already knows about the problem comes first: read the relevant documents to the end and cite them",
			"pt-BR": "nenhuma evidência vem dos documentos do próprio projeto ({docs}…). O que o projeto já sabe sobre o problema vem primeiro: leia os documentos relevantes até o fim e cite-os"},
		"diag.not_new": {
			"en":    "a new diagnosis was demanded ({why}) and it must bring evidence not cited before: the previous reading did not find the cause",
			"pt-BR": "um novo diagnóstico foi exigido ({why}) e ele precisa trazer evidência ainda não citada: a leitura anterior não achou a causa"},
		"diag.same_cause": {
			"en":    "the root cause is word for word the previous one, which the facts contradicted ({why})",
			"pt-BR": "a causa raiz é, palavra por palavra, a anterior, que os fatos contradisseram ({why})"},
		"diag.registered": {
			"en":    "diagnosis registered with {count} verified pieces of evidence; the user sees it beside the plan. Now register the plan (TodoWrite) that follows from it.",
			"pt-BR": "diagnóstico registrado com {count} evidências conferidas; o usuário o vê ao lado do plano. Agora registre o plano (TodoWrite) que decorre dele."},
		"diag.redo_notice": {
			"en":    "[infuse] New diagnosis required: {why}.",
			"pt-BR": "[infuse] Novo diagnóstico exigido: {why}."},
		"gate.redo": {
			"en":    "[infuse] Blocked: a new diagnosis is required ({why}). The current approach produced that sign; continuing it treats effects. Read further — the project's documents first, to the end — and register a new Diagnosis with evidence you did not cite before.",
			"pt-BR": "[infuse] Bloqueado: é preciso um novo diagnóstico ({why}). A abordagem atual produziu esse sinal; continuar nela trata efeitos. Leia mais — os documentos do projeto primeiro, até o fim — e registre um novo diagnóstico com evidência que você ainda não citou."},
		"gate.no_diagnosis": {
			"en":    "[infuse] Blocked: no diagnosis for this request yet. Before changing files or running anything that is not a read, find out what the project already knows: read its instructions and the documents about this problem to the end, read the code involved, and register a Diagnosis (problem, root cause that explains all symptoms, approach, why it treats the cause, verbatim evidence).",
			"pt-BR": "[infuse] Bloqueado: ainda não há diagnóstico para este pedido. Antes de alterar arquivos ou rodar algo que não seja leitura, descubra o que o projeto já sabe: leia até o fim as instruções e os documentos sobre este problema, leia o código envolvido e registre um diagnóstico (problema, causa raiz que explica todos os sintomas, abordagem, por que ela trata a causa, evidências literais)."},
		"redo.loop": {
			"en":    "`{command}` ran again after each change, {rounds} rounds, without the cause being found",
			"pt-BR": "`{command}` rodou de novo depois de cada alteração, {rounds} rodadas, sem a causa ser encontrada"},
		"redo.regression": {
			"en":    "the project's check `{command}` passed before your last change and fails after it",
			"pt-BR": "a verificação do projeto `{command}` passava antes da sua última alteração e falha depois dela"},
		"redo.revert": {
			"en":    "you returned {file} to a content it had earlier in this turn, undoing your own change",
			"pt-BR": "você devolveu {file} a um conteúdo que ele tinha antes neste turno, desfazendo a própria alteração"},
		"redo.breaker": {
			"en":    "the same call failed repeatedly, twice",
			"pt-BR": "a mesma chamada falhou repetidamente, duas vezes"},
		"knowledge.reading": {
			"en":    "[infuse] Reading the project's documents to the end before the agent acts: {count} document(s) new or changed.",
			"pt-BR": "[infuse] Lendo os documentos do projeto até o fim antes de o agente agir: {count} documento(s) novo(s) ou alterado(s)."},
		"provider.retry": {
			"en":    "[infuse] The provider failed ({error}); trying again in {seconds}s (attempt {attempt}).",
			"pt-BR": "[infuse] O provedor falhou ({error}); tentando de novo em {seconds}s (tentativa {attempt})."},
		"reply.cut": {
			"en":    "[infuse] Your reply was cut at the output limit. Continue exactly where it stopped.",
			"pt-BR": "[infuse] Sua resposta foi cortada no limite de saída. Continue exatamente de onde parou."},
		"reply.cut_calls": {
			"en":    "[infuse] Your reply was cut at the output limit. Continue exactly where it stopped. {count} tool call(s) in it were not run, because their input may be incomplete: make them again, in full (for a large file, write it in parts with Edit).",
			"pt-BR": "[infuse] Sua resposta foi cortada no limite de saída. Continue exatamente de onde parou. {count} chamada(s) de ferramenta nela não rodaram, porque a entrada pode estar incompleta: faça-as de novo, inteiras (para um arquivo grande, escreva em partes com Edit)."},
		"stall.warn": {
			"en":    "[infuse] {steps} steps without progress: no change landed and no plan task finished. Stop exploring: state in the plan (TodoWrite) the next concrete change and make it, or mark the task blocked with the reason.",
			"pt-BR": "[infuse] {steps} passos sem progresso: nenhuma alteração entrou e nenhuma tarefa do plano terminou. Pare de explorar: registre no plano (TodoWrite) a próxima alteração concreta e faça-a, ou marque a tarefa como bloqueada com o motivo."},
		"stall.stop": {
			"en":    "no progress in {steps} steps (no change landed and no plan task finished)",
			"pt-BR": "sem progresso em {steps} passos (nenhuma alteração entrou e nenhuma tarefa do plano terminou)"},
		"resume.note": {
			"en":    "[infuse] infuse was restarted while you were working, and the turn was interrupted. Continue the task exactly from where it stopped: check the plan and the files you were changing, then go on.",
			"pt-BR": "[infuse] O infuse foi reiniciado enquanto você trabalhava, e o turno foi interrompido. Continue a tarefa exatamente de onde parou: confira o plano e os arquivos que estava alterando, e siga."},
		"tool.panic": {
			"en":    "[infuse] The tool {tool} failed inside infuse ({error}). This is infuse's fault, not your input; try another way to do the same step.",
			"pt-BR": "[infuse] A ferramenta {tool} falhou dentro do infuse ({error}). A falha é do infuse, não da sua entrada; tente outra forma de fazer o mesmo passo."},
	})
}
