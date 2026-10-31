package core

// Message catalog (part 4).

func init() {
	RegisterMessages(map[string]map[string]string{
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
	})
}
