// Command go-inspect extracts architecture-relevant syntax using the Go parser.
package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

type source struct {
	File   string `json:"file"`
	Source string `json:"source"`
}
type imported struct {
	Path  string `json:"path"`
	Alias string `json:"alias"`
}
type call struct {
	Name     string `json:"name"`
	Function string `json:"function"`
	Routing  bool   `json:"routing"`
	Database bool   `json:"database"`
}
type modelType struct {
	Name      string `json:"name"`
	Persisted bool   `json:"persisted"`
	Fields    int    `json:"fields"`
}
type features struct {
	File       string      `json:"file"`
	Imports    []imported  `json:"imports"`
	Calls      []call      `json:"calls"`
	Types      []modelType `json:"types"`
	Selectors  []string    `json:"selectors"`
	RoutePaths []string    `json:"routePaths"`
	ParseError string      `json:"parseError,omitempty"`
}

func text(expr ast.Expr) string {
	if value, ok := expr.(*ast.BasicLit); ok && value.Kind == token.STRING {
		result, _ := strconv.Unquote(value.Value)
		return result
	}
	return ""
}

func selector(expr ast.Expr) (string, string) {
	switch node := expr.(type) {
	case *ast.IndexExpr:
		return selector(node.X)
	case *ast.IndexListExpr:
		return selector(node.X)
	case *ast.SelectorExpr:
		if object, ok := node.X.(*ast.Ident); ok {
			return object.Name, node.Sel.Name
		}
		return "", node.Sel.Name
	case *ast.Ident:
		return "", node.Name
	}
	return "", ""
}

func contains(values, value string) bool { return strings.Contains("|"+values+"|", "|"+value+"|") }

func inspect(input source) features {
	result := features{File: input.File}
	file, err := parser.ParseFile(token.NewFileSet(), input.File, input.Source, 0)
	if err != nil {
		result.ParseError = err.Error()
		return result
	}
	imports := map[string]string{}
	for _, item := range file.Imports {
		importPath := text(item.Path)
		alias := importPath[strings.LastIndex(importPath, "/")+1:]
		if item.Name != nil {
			alias = item.Name.Name
		}
		imports[alias] = importPath
		result.Imports = append(result.Imports, imported{Path: importPath, Alias: alias})
	}
	for _, declaration := range file.Decls {
		function := "<module>"
		if fn, ok := declaration.(*ast.FuncDecl); ok {
			function = fn.Name.Name
			if fn.Name.Name == "TableName" && fn.Recv != nil {
				result.Types = append(result.Types, modelType{Name: "TableName", Persisted: true})
			}
		}
		ast.Inspect(declaration, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.SelectorExpr:
				if value.Sel.Name == "PRISM_DB" {
					result.Selectors = append(result.Selectors, "PRISM_DB")
				}
			case *ast.CallExpr:
				object, name := selector(value.Fun)
				packagePath := imports[object]
				routing := (packagePath == "" || packagePath == "net/http" || packagePath == "github.com/gin-gonic/gin") && contains("GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|Any|Handle|HandleFunc|Static|StaticFS|StaticFile|NoRoute|NewServeMux", name) || (name == "Register" && strings.Contains(packagePath, "danielgtaylor/huma"))
				database := (packagePath == "" || strings.HasPrefix(packagePath, "gorm.io/")) && contains("AutoMigrate|Migrator|Create|Save|Delete|Updates|Update|Where|Table|Raw|Exec|First|Find|Model", name)
				if routing || database {
					result.Calls = append(result.Calls, call{Name: name, Function: function, Routing: routing, Database: database})
				}
				if routing && len(value.Args) > 0 && name != "Register" && name != "NoRoute" {
					if route := text(value.Args[0]); route != "" {
						result.RoutePaths = append(result.RoutePaths, route)
					} else {
						result.RoutePaths = append(result.RoutePaths, "<dynamic>")
					}
				}
			case *ast.CompositeLit:
				object, name := selector(value.Type)
				if name == "Operation" && strings.Contains(imports[object], "danielgtaylor/huma") {
					for _, element := range value.Elts {
						pair, ok := element.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "Path" {
							if route := text(pair.Value); route != "" {
								result.RoutePaths = append(result.RoutePaths, route)
							}
						}
					}
				}
			case *ast.TypeSpec:
				structure, ok := value.Type.(*ast.StructType)
				if !ok {
					break
				}
				item := modelType{Name: value.Name.Name, Fields: len(structure.Fields.List)}
				for _, field := range structure.Fields.List {
					if field.Tag != nil && strings.Contains(field.Tag.Value, "gorm:") {
						item.Persisted = true
					}
					_, name := selector(field.Type)
					if name == "MODEL" || name == "Model" || name == "DeletedAt" {
						item.Persisted = true
					}
				}
				result.Types = append(result.Types, item)
			}
			return true
		})
	}
	return result
}

func main() {
	var sources []source
	if err := json.NewDecoder(os.Stdin).Decode(&sources); err != nil {
		panic(err)
	}
	results := make([]features, 0, len(sources))
	for _, item := range sources {
		results = append(results, inspect(item))
	}
	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		panic(err)
	}
}
