package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

func runInit(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: kite-plugin init <name> [--with-frontend]")
	}

	name := args[0]
	if name == "" || strings.ContainsAny(name, " /\\") {
		return fmt.Errorf("invalid plugin name: %q (no spaces or slashes)", name)
	}

	withFrontend := false
	for _, a := range args[1:] {
		if a == "--with-frontend" {
			withFrontend = true
		}
	}

	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}

	if err := os.MkdirAll(name, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data := scaffoldData{
		Name:         name,
		NameTitle:    toTitle(name),
		WithFrontend: withFrontend,
	}

	// Backend files
	files := []scaffoldFile{
		{Path: "main.go", Tmpl: mainGoTmpl},
		{Path: "manifest.yaml", Tmpl: manifestYamlTmpl},
		{Path: "go.mod", Tmpl: goModTmpl},
		{Path: "Makefile", Tmpl: makefileTmpl},
		{Path: "README.md", Tmpl: readmeTmpl},
	}

	// Frontend files
	if withFrontend {
		files = append(files,
			scaffoldFile{Path: "frontend/package.json", Tmpl: frontendPackageJsonTmpl},
			scaffoldFile{Path: "frontend/vite.config.ts", Tmpl: frontendViteConfigTmpl},
			scaffoldFile{Path: "frontend/tsconfig.json", Tmpl: frontendTsconfigTmpl},
			scaffoldFile{Path: "frontend/src/PluginPage.tsx", Tmpl: frontendPluginPageTmpl},
			scaffoldFile{Path: "frontend/src/Settings.tsx", Tmpl: frontendSettingsTmpl},
		)
	}

	for _, f := range files {
		if err := writeTemplate(filepath.Join(name, f.Path), f.Tmpl, data); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
	}

	fmt.Printf("✓ Plugin %q created successfully\n", name)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  go mod tidy\n")
	if withFrontend {
		fmt.Printf("  cd frontend && pnpm install && cd ..\n")
	}
	fmt.Printf("  kite-plugin build\n")

	return nil
}

type scaffoldData struct {
	Name         string
	NameTitle    string
	WithFrontend bool
}

type scaffoldFile struct {
	Path string
	Tmpl string
}

func writeTemplate(path, tmplStr string, data scaffoldData) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, data)
}

func toTitle(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}
