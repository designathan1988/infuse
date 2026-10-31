package main

// System prompt assembly: identity, environment, the project's knowledge (map, rules, lessons, oracle),
// and layered instructions kept for compatibility with existing CLAUDE.md / AGENTS.md files.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func buildSystemPrompt(s *Session, rules *RuleSet) string {
	var b strings.Builder
	b.WriteString(responseLanguageRule(s.uiLang()) + "\n\n")
	b.WriteString("Você é o agente de engenharia de software do infuse Studio, trabalhando diretamente no projeto do usuário com as ferramentas disponíveis.\n\n")
	b.WriteString("Princípios de trabalho:\n")
	b.WriteString("- Compreenda antes de agir: leia os arquivos envolvidos e o mapa do projeto; identifique dependentes antes de mudar algo.\n")
	b.WriteString("- Construa dentro da estrutura existente do projeto. Não espalhe responsabilidades; não crie condicionais onde existe um ponto de extensão.\n")
	b.WriteString("- Mudanças mínimas e precisas: use Edit para alterações pontuais; não reescreva arquivos funcionais sem necessidade; não remova validações ou verificações de segurança.\n")
	b.WriteString("- Nunca modifique testes ou critérios para fazê-los passar; nunca silencie erros (catch vazio, valores padrão que escondem falhas).\n")
	b.WriteString("- Verifique executando (build, testes, o próprio programa) e relate o resultado real. Se não conseguir atingir o objetivo, diga exatamente o que tentou, a evidência e onde parou — isso é um resultado válido.\n")
	b.WriteString("- Consulte documentação oficial da versão instalada antes de tentativa e erro.\n")
	b.WriteString("- Responda em português, de forma direta.\n\n")

	b.WriteString("Protocolo do infuse (aplicado pelo infuse a cada ação; uma ação fora dele é recusada com o motivo):\n")
	b.WriteString("1. Antes de alterar arquivos: registre o plano com TodoWrite, cada tarefa aberta com proof.\n")
	b.WriteString("2. Antes de escrever código que usa uma biblioteca ou API externa: consulte a documentação oficial da versão em uso neste turno (WebSearch, WebFetch, documentação fixada, código do pacote).\n")
	b.WriteString("3. O código segue as convenções do projeto e o limite de tamanho de arquivo.\n")
	b.WriteString("4. Duas falhas seguidas exigem investigação antes de nova tentativa.\n")
	b.WriteString("5. O turno só termina com o plano concluído (done ou blocked), o verificador do projeto sem erros nos arquivos alterados, um Report conferido e os achados da revisão independente tratados.\n\n")
	fmt.Fprintf(&b, "Ambiente: %s/%s · data %s · diretório do projeto: %s · diretório atual: %s\n", runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"), s.Root, s.Cwd)
	if branch := gitOutput(s.Root, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		fmt.Fprintf(&b, "Git: branch %s", branch)
		if st := gitOutput(s.Root, "status", "--porcelain"); st != "" {
			fmt.Fprintf(&b, " · %d arquivo(s) com alterações", len(strings.Split(strings.TrimSpace(st), "\n")))
		}
		b.WriteString("\n")
	}
	if s.mode() == "plan" {
		b.WriteString("MODO PLANO: você pode ler e investigar, mas escrita e execução estão desativadas. Produza um plano para aprovação.\n")
	}

	// The project's knowledge — the grounds of the filter.
	if m := LoadOrBuildMap(s.Root); m != nil {
		b.WriteString("\n## Mapa do projeto (gerado do código)\n")
		b.WriteString(DescribeDir(m, ".") + "\n")
		if len(m.Hotspots) > 0 {
			b.WriteString("Arquivos grandes demais (evite acrescentar responsabilidades a eles):\n")
			for i, h := range m.Hotspots {
				if i >= 8 {
					break
				}
				fmt.Fprintf(&b, "- %s (%d linhas)\n", h.Path, h.Lines)
			}
		}
		b.WriteString("Use LS em qualquer pasta para ver o mapa dela; ao escrever um arquivo você recebe quem depende dele.\n")
	}
	if oracle := readOracle(s.Root); oracle != "" {
		b.WriteString("\n## Critérios de pronto (definidos pelo usuário; somente leitura — você não pode alterá-los)\n" + oracle + "\n")
	}
	if lines := alwaysRuleTexts(s.Root, rules); len(lines) > 0 {
		b.WriteString("\n## Regras do projeto\n" + strings.Join(lines, "\n") + "\n")
	}
	if l := lessonsSummary(s.Root); l != "" {
		b.WriteString("\n## Vícios já observados neste projeto (não repita)\n" + l + "\n")
	}
	if lock := instructionsPrompt(s.Root); lock != "" {
		b.WriteString(lock + "\n")
	}
	if inst := loadInstructions(s.Root); inst != "" {
		b.WriteString("\n## Instruções do usuário e do projeto\n" + inst + "\n")
	}
	if mem := readIfExists(filepath.Join(s.Root, infuseDirName, "memory.md")); mem != "" {
		b.WriteString("\n## Memória do projeto\n" + mem + "\n")
	}
	if c := conventionsPrompt(s.Root); c != "" {
		b.WriteString("\n## Convenções do projeto (siga ao escrever código; .infuse/conventions.md)\n" + c + "\n")
	}
	if d := docsPrompt(s.Root); d != "" {
		b.WriteString("\n## Documentação fixada pelo usuário (leia com Read antes de usar estas bibliotecas)\n" + d)
	}
	if sk := skillsPrompt(s.Root); sk != "" {
		b.WriteString("\n## Skills disponíveis (carregue com a ferramenta Skill quando a tarefa corresponder)\n" + sk)
	}
	if items := memoryPrompt(s); items != "" {
		b.WriteString("\n## Memória estruturada (mais importante primeiro; registre e remova com a ferramenta Memory)\n" + items)
	}
	return b.String()
}

func gitOutput(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	noConsoleWindow(cmd)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readIfExists(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readOracle(root string) string {
	dir := filepath.Join(root, infuseDirName, "oracle")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var parts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if t := readIfExists(filepath.Join(dir, e.Name())); t != "" {
			parts = append(parts, "### "+e.Name()+"\n"+t)
		}
	}
	return strings.Join(parts, "\n\n")
}

func alwaysRuleTexts(root string, set *RuleSet) []string {
	var out []string
	for _, r := range set.Rules {
		if r.When != "always" {
			continue
		}
		out = append(out, r.expand(root)...)
	}
	return out
}

func lessonsSummary(root string) string {
	list := readJSONL(filepath.Join(root, infuseDirName, "lessons.jsonl"))
	if len(list) == 0 {
		return ""
	}
	count := map[string]int{}
	for _, e := range list {
		if v, ok := e["vice"].(string); ok {
			count[v]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range count {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v })
	var parts []string
	for i, e := range kvs {
		if i >= 10 {
			break
		}
		parts = append(parts, fmt.Sprintf("- %s (%d vez(es))", e.k, e.v))
	}
	return strings.Join(parts, "\n")
}

// loadInstructions: global, then project; compatible with CLAUDE.md and AGENTS.md.
func loadInstructions(root string) string {
	var parts []string
	for _, p := range []string{
		filepath.Join(InfuseHome(), "instructions.md"),
		filepath.Join(homeDir(), ".claude", "CLAUDE.md"),
		filepath.Join(root, infuseDirName, "instructions.md"),
		filepath.Join(root, "CLAUDE.md"),
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, ".claude", "CLAUDE.md"),
	} {
		t := readIfExists(p)
		if t == "" && strings.HasPrefix(p, root) { // deleted from the working tree: the committed version still rules
			if rel, err := filepath.Rel(root, p); err == nil && contains(loadProjectRules(root).Missing, filepath.ToSlash(rel)) {
				t = gitHeadFile(root, filepath.ToSlash(rel))
			}
		}
		if t != "" {
			parts = append(parts, "<!-- "+p+" -->\n"+resolveInstructionImports(root, filepath.Dir(p), t, 0))
		}
	}
	return strings.Join(parts, "\n\n")
}
