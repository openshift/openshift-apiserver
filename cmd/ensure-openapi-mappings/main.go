package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/openshift/openshift-apiserver/pkg/openapi"
	"k8s.io/kube-openapi/pkg/util"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

var (
	updateFlag  *bool
	openapiFile *string
)

type validationResult struct {
	githubComCount int
	comGithubCount int
	missingMap     map[string]string
	extraComGithub []string
	passed         bool
}

func init() {
	updateFlag = flag.Bool("update", false, "Update the OpenAPI definitions file to add missing com.github.openshift mappings")
	openapiFile = flag.String("openapi-file", "", "Path to the OpenAPI definitions file (required)")
}

func validateFlags(openapiFilePath string) {
	if openapiFilePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --openapi-file flag is required\n")
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	flag.Parse()
	validateFlags(*openapiFile)
	result := validateOpenAPISchemas()

	// Print statistics
	fmt.Printf("OpenAPI Schema Validation\n")
	fmt.Printf("==========================\n")
	fmt.Printf("github.com/openshift keys: %d\n", result.githubComCount)
	fmt.Printf("com.github.openshift keys: %d\n", result.comGithubCount)
	fmt.Printf("Counts match: %v\n", result.githubComCount == result.comGithubCount)
	fmt.Printf("Missing mappings: %d\n", len(result.missingMap))
	fmt.Printf("Extra com.github keys: %d\n", len(result.extraComGithub))

	if result.passed {
		fmt.Printf("\nStatus: PASS - Perfect 1:1 mapping\n")
		os.Exit(0)
	}

	// Validation failed
	if *updateFlag {
		if len(result.missingMap) > 0 {
			fmt.Printf("\nUpdating %s to add %d missing mappings...\n", *openapiFile, len(result.missingMap))
			if err := updateOpenAPIFile(*openapiFile, result.missingMap); err != nil {
				fmt.Fprintf(os.Stderr, "Error updating file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully updated %s\n", *openapiFile)
			fmt.Printf("\nNote: Run 'make update-gofmt' to format the updated file.\n")
			os.Exit(0)
		} else {
			fmt.Printf("\nNo missing mappings to add, but validation still failed.\n")
			os.Exit(1)
		}
	}

	// Validation failed - print detailed information
	fmt.Printf("\nStatus: FAIL - Mapping validation failed\n")
	fmt.Printf("\nRun with --update flag to automatically add missing mappings.\n\n")

	if len(result.missingMap) > 0 {
		fmt.Printf("Missing com.github.openshift mappings (%d total):\n", len(result.missingMap))
		fmt.Printf("==================================================\n")

		// Extract and sort missing keys for consistent output
		var missingKeys []string
		for _, githubKey := range result.missingMap {
			missingKeys = append(missingKeys, githubKey)
		}
		sort.Strings(missingKeys)

		for i, key := range missingKeys {
			if i >= 20 {
				fmt.Printf("... and %d more\n", len(result.missingMap)-20)
				break
			}
			expectedComKey := util.ToRESTFriendlyName(key)
			fmt.Printf("  %s\n  (expected: %s)\n\n", key, expectedComKey)
		}
	}

	if len(result.extraComGithub) > 0 {
		fmt.Printf("\nExtra com.github.openshift keys without github.com counterpart (%d total):\n", len(result.extraComGithub))
		fmt.Printf("===========================================================================\n")
		for i, key := range result.extraComGithub {
			if i >= 20 {
				fmt.Printf("... and %d more\n", len(result.extraComGithub)-20)
				break
			}
			fmt.Printf("  %s\n", key)
		}
	}

	os.Exit(1)
}

func validateOpenAPISchemas() validationResult {
	// Get all OpenAPI definitions
	definitions := openapi.GetOpenAPIDefinitions(func(path string) spec.Ref {
		return spec.MustCreateRef("#/definitions/" + path)
	})

	// Filter and collect keys with both prefixes
	var githubComKeys []string
	var comGithubKeys []string
	comGithubMap := make(map[string]bool)

	for key := range definitions {
		if strings.HasPrefix(key, "github.com/openshift") {
			githubComKeys = append(githubComKeys, key)
		} else if strings.HasPrefix(key, "com.github.openshift") {
			comGithubKeys = append(comGithubKeys, key)
			comGithubMap[key] = true
		}
	}

	// Sort keys for consistent output
	sort.Strings(githubComKeys)
	sort.Strings(comGithubKeys)

	// Validate mapping and build expected map
	missingMap := make(map[string]string)         // comKey -> githubKey
	expectedComGithubMap := make(map[string]bool) // all expected com.github keys

	for _, githubKey := range githubComKeys {
		// Convert to REST-friendly name (github.com/openshift/... -> com.github.openshift....)
		comKey := util.ToRESTFriendlyName(githubKey)
		expectedComGithubMap[comKey] = true

		if !comGithubMap[comKey] {
			missingMap[comKey] = githubKey
		}
	}

	var extraComGithub []string
	for _, comKey := range comGithubKeys {
		if !expectedComGithubMap[comKey] {
			extraComGithub = append(extraComGithub, comKey)
		}
	}

	// Determine if validation passed
	validationPassed := len(missingMap) == 0 && len(extraComGithub) == 0 && len(githubComKeys) == len(comGithubKeys)

	return validationResult{
		githubComCount: len(githubComKeys),
		comGithubCount: len(comGithubKeys),
		missingMap:     missingMap,
		extraComGithub: extraComGithub,
		passed:         validationPassed,
	}
}

func updateOpenAPIFile(openapiFile string, missingMap map[string]string) error {
	// Parse the file
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, openapiFile, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	// Find the GetOpenAPIDefinitions function
	var funcDecl *ast.FuncDecl
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "GetOpenAPIDefinitions" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		return fmt.Errorf("GetOpenAPIDefinitions function not found")
	}

	// Find the return statement with the map
	var mapLit *ast.CompositeLit
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok {
			if len(ret.Results) > 0 {
				if comp, ok := ret.Results[0].(*ast.CompositeLit); ok {
					mapLit = comp
					return false
				}
			}
		}
		return true
	})

	if mapLit == nil {
		return fmt.Errorf("map literal not found in GetOpenAPIDefinitions")
	}

	// Build a map of existing string-keyed entries
	existingFuncs := make(map[string]ast.Expr) // key -> value expression
	comGithubIndices := make(map[int]bool)     // indices of com.github.openshift entries to remove

	for i, elt := range mapLit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if keyLit, ok := kv.Key.(*ast.BasicLit); ok && keyLit.Kind == token.STRING {
				key := strings.Trim(keyLit.Value, "\"")
				existingFuncs[key] = kv.Value
				if strings.HasPrefix(key, "com.github.openshift") {
					comGithubIndices[i] = true
				}
			}
		}
	}

	// Build complete list of all com.github.openshift entries
	type newEntry struct {
		key   string
		value ast.Expr
	}
	var allComGithubEntries []newEntry

	// Add existing com.github.openshift entries (keep them in the regenerated list)
	for key, value := range existingFuncs {
		if strings.HasPrefix(key, "com.github.openshift") {
			allComGithubEntries = append(allComGithubEntries, newEntry{
				key:   key,
				value: value,
			})
		}
	}

	// Add missing entries
	for comKey, githubKey := range missingMap {
		if valueExpr, ok := existingFuncs[githubKey]; ok {
			allComGithubEntries = append(allComGithubEntries, newEntry{
				key:   comKey,
				value: valueExpr,
			})
		}
	}

	// Sort all com.github.openshift entries by key
	sort.Slice(allComGithubEntries, func(i, j int) bool {
		return allComGithubEntries[i].key < allComGithubEntries[j].key
	})

	// Rebuild the map elements: keep all entries EXCEPT com.github.openshift, then insert sorted com.github entries
	var newElts []ast.Expr
	var lastGithubIndex = -1

	// Iterate through all existing map elements
	for i, elt := range mapLit.Elts {
		// Skip com.github.openshift entries (we'll add them sorted later)
		if comGithubIndices[i] {
			continue
		}

		// Keep this element
		newElts = append(newElts, elt)

		// Check if this is the last github.com/openshift entry
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if keyLit, ok := kv.Key.(*ast.BasicLit); ok && keyLit.Kind == token.STRING {
				key := strings.Trim(keyLit.Value, "\"")
				if strings.HasPrefix(key, "github.com/openshift") {
					lastGithubIndex = len(newElts) - 1
				}
			}
		}
	}

	// Insert all com.github.openshift entries after the last github.com/openshift entry
	if lastGithubIndex != -1 {
		insertPos := lastGithubIndex + 1

		var comGithubElts []ast.Expr
		for _, entry := range allComGithubEntries {
			kv := &ast.KeyValueExpr{
				Key: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf("%q", entry.key),
				},
				Value: entry.value,
			}
			comGithubElts = append(comGithubElts, kv)
		}
		// Insert at position
		newElts = append(newElts[:insertPos], append(comGithubElts, newElts[insertPos:]...)...)
	}

	// Update the map literal
	mapLit.Elts = newElts

	// Write the updated AST to a buffer first
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&buf, fset, node); err != nil {
		return fmt.Errorf("failed to print AST: %w", err)
	}

	// Write to file
	if err := os.WriteFile(openapiFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
