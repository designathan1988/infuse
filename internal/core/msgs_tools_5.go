package core

// Message catalog (part 5).

func init() {
	RegisterMessages(map[string]map[string]string{
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
