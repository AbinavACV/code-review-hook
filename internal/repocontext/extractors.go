package repocontext

import (
	"fmt"
	"strings"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspy "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// extractor produces a skeleton (signatures, types, exported decls) from src.
type extractor func(src []byte) (string, error)

// symbolDetector returns top-level symbol names declared in src.
type symbolDetector func(src []byte) []string

// langSpec bundles everything needed to skeletonize one language.
type langSpec struct {
	name      string
	language  func() unsafe.Pointer
	skeletonQ string
	symbolQ   string
	// captures from skeletonQ that should be emitted as a one-line signature.
	// Lower priority captures replace higher ones in case of overlap.
	signatureCaps []string
	// nameCap is the capture that yields the symbol name within a definition.
	nameCap string
	// keepCap is the capture that holds the full node to emit (signature only,
	// body stripped via `body:` field where applicable).
	keepCap string
}

var langs = map[string]*langSpec{
	".go": {
		name:     "go",
		language: tsgo.Language,
		skeletonQ: `
(function_declaration name: (identifier) @name) @def
(method_declaration name: (field_identifier) @name) @def
(type_declaration) @def
(const_declaration) @def
(var_declaration) @def
`,
		symbolQ: `
(function_declaration name: (identifier) @name)
(method_declaration name: (field_identifier) @name)
(type_spec name: (type_identifier) @name)
(const_spec name: (identifier) @name)
(var_spec name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
	".py": {
		name:     "python",
		language: tspy.Language,
		skeletonQ: `
(function_definition name: (identifier) @name) @def
(class_definition name: (identifier) @name) @def
`,
		symbolQ: `
(function_definition name: (identifier) @name)
(class_definition name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
	".js": {
		name:     "javascript",
		language: tsjs.Language,
		skeletonQ: `
(function_declaration name: (identifier) @name) @def
(class_declaration name: (identifier) @name) @def
(method_definition name: (property_identifier) @name) @def
(lexical_declaration (variable_declarator name: (identifier) @name value: [(arrow_function) (function_expression)])) @def
`,
		symbolQ: `
(function_declaration name: (identifier) @name)
(class_declaration name: (identifier) @name)
(method_definition name: (property_identifier) @name)
(variable_declarator name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
	".jsx": {
		name:     "javascript",
		language: tsjs.Language,
		skeletonQ: `
(function_declaration name: (identifier) @name) @def
(class_declaration name: (identifier) @name) @def
(method_definition name: (property_identifier) @name) @def
(lexical_declaration (variable_declarator name: (identifier) @name value: [(arrow_function) (function_expression)])) @def
`,
		symbolQ: `
(function_declaration name: (identifier) @name)
(class_declaration name: (identifier) @name)
(method_definition name: (property_identifier) @name)
(variable_declarator name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
	".ts": {
		name:     "typescript",
		language: tsts.LanguageTypescript,
		skeletonQ: `
(function_declaration name: (identifier) @name) @def
(class_declaration name: (type_identifier) @name) @def
(method_definition name: (property_identifier) @name) @def
(interface_declaration name: (type_identifier) @name) @def
(type_alias_declaration name: (type_identifier) @name) @def
(lexical_declaration (variable_declarator name: (identifier) @name)) @def
`,
		symbolQ: `
(function_declaration name: (identifier) @name)
(class_declaration name: (type_identifier) @name)
(method_definition name: (property_identifier) @name)
(interface_declaration name: (type_identifier) @name)
(type_alias_declaration name: (type_identifier) @name)
(variable_declarator name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
	".tsx": {
		name:     "tsx",
		language: tsts.LanguageTSX,
		skeletonQ: `
(function_declaration name: (identifier) @name) @def
(class_declaration name: (type_identifier) @name) @def
(method_definition name: (property_identifier) @name) @def
(interface_declaration name: (type_identifier) @name) @def
(type_alias_declaration name: (type_identifier) @name) @def
(lexical_declaration (variable_declarator name: (identifier) @name)) @def
`,
		symbolQ: `
(function_declaration name: (identifier) @name)
(class_declaration name: (type_identifier) @name)
(method_definition name: (property_identifier) @name)
(interface_declaration name: (type_identifier) @name)
(type_alias_declaration name: (type_identifier) @name)
(variable_declarator name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
	".rs": {
		name:     "rust",
		language: tsrust.Language,
		skeletonQ: `
(function_item name: (identifier) @name) @def
(struct_item name: (type_identifier) @name) @def
(enum_item name: (type_identifier) @name) @def
(trait_item name: (type_identifier) @name) @def
(impl_item) @def
(const_item name: (identifier) @name) @def
`,
		symbolQ: `
(function_item name: (identifier) @name)
(struct_item name: (type_identifier) @name)
(enum_item name: (type_identifier) @name)
(trait_item name: (type_identifier) @name)
(const_item name: (identifier) @name)
`,
		nameCap: "name",
		keepCap: "def",
	},
}

// extractorFor returns (extractor, language-name) for the file extension,
// or (nil, "") if unsupported. languageName is the friendly label for output.
func extractorFor(ext string) (extractor, string) {
	spec, ok := langs[ext]
	if !ok {
		return nil, languageNameForExt(ext)
	}
	return func(src []byte) (string, error) {
		return runSkeletonQuery(spec, src)
	}, spec.name
}

func symbolDetectorFor(ext string) symbolDetector {
	spec, ok := langs[ext]
	if !ok {
		return nil
	}
	return func(src []byte) []string {
		return runSymbolQuery(spec, src)
	}
}

func languageNameForExt(ext string) string {
	if spec, ok := langs[ext]; ok {
		return spec.name
	}
	if ext == "" {
		return "no extension"
	}
	return strings.TrimPrefix(ext, ".")
}

func supportedExts() []string {
	out := make([]string, 0, len(langs))
	for ext := range langs {
		out = append(out, ext)
	}
	return out
}

// runSkeletonQuery walks the file, emitting one line per top-level definition.
// For declarations with a body, only the prefix up to (and excluding) the body
// is emitted — that's the signature.
func runSkeletonQuery(spec *langSpec, src []byte) (string, error) {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	lang := tree_sitter.NewLanguage(spec.language())
	if err := parser.SetLanguage(lang); err != nil {
		return "", fmt.Errorf("set language: %w", err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return "", fmt.Errorf("parse returned nil tree")
	}
	defer tree.Close()

	q, qerr := tree_sitter.NewQuery(lang, spec.skeletonQ)
	if qerr != nil {
		return "", fmt.Errorf("query: %v", qerr)
	}
	defer q.Close()

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	var lines []string
	matches := cursor.Matches(q, tree.RootNode(), src)
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		var defNode *tree_sitter.Node
		for _, c := range m.Captures {
			if q.CaptureNames()[c.Index] == spec.keepCap {
				node := c.Node
				defNode = &node
			}
		}
		if defNode == nil {
			continue
		}
		sig := signatureFor(defNode, src)
		if sig == "" {
			continue
		}
		lines = append(lines, sig)
	}
	return strings.Join(lines, "\n"), nil
}

// signatureFor returns the source slice of node up to (but not including) its
// body, if it has one. For nodes without a separable body (types, consts), the
// full text is returned, single-lined.
func signatureFor(node *tree_sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		// Fallback heuristic: if there's a child with kind block / statement_block /
		// declaration_list, drop everything from there.
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "block", "statement_block", "declaration_list", "field_declaration_list":
				return collapseWhitespace(string(src[node.StartByte():child.StartByte()]))
			}
		}
		return collapseWhitespace(string(src[node.StartByte():node.EndByte()]))
	}
	return collapseWhitespace(string(src[node.StartByte():body.StartByte()]))
}

func collapseWhitespace(s string) string {
	s = strings.TrimSpace(s)
	// Collapse runs of whitespace (incl. newlines) to single space.
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// runSymbolQuery returns top-level symbol names from src.
func runSymbolQuery(spec *langSpec, src []byte) []string {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	lang := tree_sitter.NewLanguage(spec.language())
	if err := parser.SetLanguage(lang); err != nil {
		return nil
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	q, qerr := tree_sitter.NewQuery(lang, spec.symbolQ)
	if qerr != nil {
		return nil
	}
	defer q.Close()

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	var names []string
	matches := cursor.Matches(q, tree.RootNode(), src)
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		for _, c := range m.Captures {
			if q.CaptureNames()[c.Index] != spec.nameCap {
				continue
			}
			name := c.Node.Utf8Text(src)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}
