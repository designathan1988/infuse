package main

// Data in JSON has a contract: a data file points to its JSON Schema ("$schema") and is validated against
// it before it is written; a schema is checked to compile. Configuration files of tools (package.json,
// tsconfig…) are not data. Validation: github.com/santhosh-tekuri/jsonschema/v6 (passes the official
// JSON-Schema-Test-Suite for 2020-12).

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var toolConfigJSON = map[string]bool{"package.json": true, "package-lock.json": true, "tsconfig.json": true, "jsconfig.json": true, "composer.json": true, "manifest.json": true, "launch.json": true, "settings.json": true, "tasks.json": true, "extensions.json": true, ".eslintrc.json": true, ".prettierrc.json": true, "deno.json": true, "vercel.json": true, "firebase.json": true, "app.json": true, "lerna.json": true, "nx.json": true, "turbo.json": true, "biome.json": true, "renovate.json": true}

// dataDirs are folders whose JSON files are the application's data.
var dataDirs = []string{"data/", "content/", "fixtures/", "db/", "seeds/", "public/data/", "src/data/"}

func isToolConfig(rel string) bool {
	base := path.Base(rel)
	return toolConfigJSON[base] || strings.HasPrefix(base, "tsconfig") || strings.Contains(base, ".config.") || strings.HasPrefix(base, ".")
}

// archJSON validates a data file against its schema, requires one for data folders, and checks schemas.
func archJSON(r *ArchRules, work, rel, ext, after string) string {
	if ext != ".json" || !r.on("json") || isToolConfig(rel) {
		return ""
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(after))
	if err != nil {
		return "" // a JSON syntax error is reported by the checker after the write
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	abs := filepath.Join(work, filepath.FromSlash(rel))
	if strings.HasSuffix(rel, ".schema.json") {
		url := fileURL(abs)
		if err := c.AddResource(url, inst); err != nil {
			return fmt.Sprintf("invalid schema in %s: %s", rel, err.Error())
		}
		if _, err := c.Compile(url); err != nil {
			return fmt.Sprintf("invalid schema in %s: %s", rel, firstLine(err.Error()))
		}
		return ""
	}
	obj, _ := inst.(map[string]any)
	ref, _ := obj["$schema"].(string)
	if ref == "" {
		inData := false
		for _, d := range dataDirs {
			if strings.HasPrefix(rel, d) || strings.Contains(rel, "/"+d) {
				inData = true
			}
		}
		if inData || len(r.SchemaDirs) > 0 {
			where := "schemas/"
			if len(r.SchemaDirs) > 0 {
				where = r.SchemaDirs[0] + "/"
			}
			return fmt.Sprintf("JSON data without a contract (%s). First write the schema in %s<name>.schema.json (JSON Schema 2020-12: $schema, $id, type, required, additionalProperties:false) and point to it with \"$schema\" in this file; the data is validated before every write.", rel, where)
		}
		return ""
	}
	if strings.HasPrefix(ref, "http://json-schema.org") || strings.HasPrefix(ref, "https://json-schema.org") {
		return "" // a meta-schema: this file is itself a schema
	}
	schemaURL := ref
	if !strings.Contains(ref, "://") {
		schemaURL = fileURL(filepath.Join(filepath.Dir(abs), filepath.FromSlash(ref)))
	} else if !strings.HasPrefix(ref, "file:") {
		return "" // a remote schema is not fetched to write a file
	}
	sch, err := c.Compile(schemaURL)
	if err != nil {
		return fmt.Sprintf("the schema that %s points to (%s) could not be loaded: %s", rel, ref, firstLine(err.Error()))
	}
	delete(obj, "$schema") // the pointer is not part of the data
	if err := sch.Validate(inst); err != nil {
		msg, _ := json.Marshal(err.Error())
		return fmt.Sprintf("the data in %s does not follow the schema %s: %s. Fix the data, or change the schema first if the contract changed.", rel, ref, strings.Trim(string(msg), `"`))
	}
	return ""
}

func fileURL(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}
