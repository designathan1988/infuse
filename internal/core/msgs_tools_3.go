package core

// Message catalog (part 3).

func init() {
	RegisterMessages(map[string]map[string]string{
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
	})
}
