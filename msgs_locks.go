package main

// Catalog: the locks of locks_cheat.go, locks_work.go and bans.go.

func init() {
	registerMessages(map[string]map[string]string{
		"cheat.swallowed_exit": {
			"en":    "[infuse] Blocked: this test command hides its own failure (`|| true`, `; exit 0` or `set +e`): `{command}`. A test run is judged by its real exit code — run it as it is and fix what fails.",
			"pt-BR": "[infuse] Bloqueado: este comando de teste esconde a própria falha (`|| true`, `; exit 0` ou `set +e`): `{command}`. Um teste é julgado pelo código de saída real — rode como está e corrija o que falhar."},
		"cheat.test_infra": {
			"en":    "[infuse] Blocked: {file} configures how the tests run and what counts as passing. Changing it to make tests pass is gaming the check. If it truly must change, say why to the user; they decide.",
			"pt-BR": "[infuse] Bloqueado: {file} configura como os testes rodam e o que conta como aprovado. Alterá-lo para os testes passarem é burlar a verificação. Se precisar mesmo mudar, diga ao usuário por quê; ele decide."},
		"cheat.always_equal": {
			"en":    "[infuse] Blocked: {file} gets an equality that always answers the same. That makes every comparison pass without checking anything.",
			"pt-BR": "[infuse] Bloqueado: {file} ganha uma igualdade que sempre responde igual. Isso faz toda comparação passar sem verificar nada."},
		"cheat.clock_patch": {
			"en":    "[infuse] Blocked: {file} replaces a clock function (Date.now, performance.now, time.time…) outside the tests. Measurements would report what the code chose, not what happened.",
			"pt-BR": "[infuse] Bloqueado: {file} substitui uma função de relógio (Date.now, performance.now, time.time…) fora dos testes. As medições passariam a mostrar o que o código escolheu, não o que aconteceu."},
		"cheat.markers": {
			"en":    "[infuse] Blocked: {count} new TODO/FIXME/HACK/XXX marker(s) in {file}. Unfinished work is not delivered as code: finish it, or register it in the plan as blocked with the reason.",
			"pt-BR": "[infuse] Bloqueado: {count} marcador(es) TODO/FIXME/HACK/XXX novo(s) em {file}. Trabalho inacabado não se entrega como código: termine, ou registre no plano como bloqueado com o motivo."},
		"cheat.stub": {
			"en":    "[infuse] Blocked: the function {function} in {file} had {before} lines and would become an empty stub (pass, return nothing, throw not-implemented). Replacing an implementation with a stub hides the work instead of doing it.",
			"pt-BR": "[infuse] Bloqueado: a função {function} em {file} tinha {before} linhas e viraria um esboço vazio (pass, retorno vazio, erro de não implementado). Trocar uma implementação por um esboço esconde o trabalho em vez de fazê-lo."},
		"cheat.special_case": {
			"en":    "[infuse] Blocked: {file} would compare inputs with the exact values the tests use ({values}, from {tests}). That makes the tests pass for those values only. Implement the general rule.",
			"pt-BR": "[infuse] Bloqueado: {file} compararia entradas com os valores exatos que os testes usam ({values}, de {tests}). Isso faz os testes passarem só para esses valores. Implemente a regra geral."},
		"ui.no_screenshot": {
			"en":    "[infuse] The turn cannot end: it changed what a screen shows ({files}) and nobody looked at it afterwards. Open it with the Browser tool, take a screenshot of each changed screen and state, look at the images, and fix what is wrong before reporting.",
			"pt-BR": "[infuse] O turno não pode terminar: ele mudou o que uma tela mostra ({files}) e ninguém olhou depois. Abra com a ferramenta Browser, fotografe cada tela e estado alterados, olhe as imagens e corrija o que estiver errado antes de relatar."},
		"work.large_removal": {
			"en":    "[infuse] Blocked: this change removes {removed} of the {total} lines of {file}. Removing most of a file must be part of the plan or the diagnosis (name the file there and why), so a feature is never cut to make a problem disappear.",
			"pt-BR": "[infuse] Bloqueado: esta alteração remove {removed} das {total} linhas de {file}. Remover a maior parte de um arquivo precisa estar no plano ou no diagnóstico (cite o arquivo e o porquê), para que um recurso nunca seja cortado só para um problema sumir."},
		"pkg.missing": {
			"en":    "[infuse] Blocked: these packages do not exist in their registry: {packages}. The name was probably invented; find the real package in the official documentation.",
			"pt-BR": "[infuse] Bloqueado: estes pacotes não existem no registro: {packages}. O nome provavelmente foi inventado; encontre o pacote real na documentação oficial."},
		"pkg.fresh": {
			"en":    "[infuse] Blocked: these packages were published less than 30 days ago: {packages}. Names invented by models get registered by attackers; confirm in the official documentation that this is the package meant, and ask the user before installing it.",
			"pt-BR": "[infuse] Bloqueado: estes pacotes foram publicados há menos de 30 dias: {packages}. Nomes inventados por modelos acabam registrados por atacantes; confirme na documentação oficial que é o pacote certo e pergunte ao usuário antes de instalar."},
		"ban.refused": {
			"en":    "[infuse] Blocked by the user's prohibition (turn {turn}): “{quote}”. It holds until the user lifts it.",
			"pt-BR": "[infuse] Bloqueado pela proibição do usuário (turno {turn}): “{quote}”. Vale até o usuário retirá-la."},
		"claims.unfetched": {
			"en":    "[infuse] The turn cannot end: you cite web addresses you never fetched nor saw in a search result: {urls}. Fetch them and check what they say, or remove them.",
			"pt-BR": "[infuse] O turno não pode terminar: você cita endereços da web que nunca buscou nem viu num resultado de pesquisa: {urls}. Busque e confira o que dizem, ou retire."},
	})
}
