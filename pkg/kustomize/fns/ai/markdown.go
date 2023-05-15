package ai

import (
	"fmt"
	"strings"

	"github.com/go-logr/zapr"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.uber.org/zap"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// MarkdownToYAML looks for an codeblocks in a markdown string containing YAML and converts them to YAML nodes.
func MarkdownToYAML(input string) ([]*yaml.RNode, error) {
	log := zapr.NewLogger(zap.L())
	// N.B. I have no idea what these options are; I just copied
	// the snippet from the docs.
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
	source := []byte(input)
	doc := md.Parser().Parse(text.NewReader(source))

	// N.B. the AST nodes store pointers into the source text. So most methods to get actual values from the
	// AST take source text as an argument.
	stack := []ast.Node{doc}

	yDocs := make([]*yaml.RNode, 0, 5)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.HasChildren() {
			stack = append(stack, n.FirstChild())
		}

		sibling := n.NextSibling()
		if sibling != nil {
			stack = append(stack, sibling)
		}

		if n.Kind() == ast.KindFencedCodeBlock {
			codeBlock := n.(*ast.FencedCodeBlock)
			language := string(codeBlock.Language(source))

			if language == "yaml" || language == "yml" || language == "" {
				lines := n.Lines()

				contents := make([]string, 0, lines.Len())
				for i := 0; i < lines.Len(); i += 1 {
					seg := lines.At(i)
					contents = append(contents, string(seg.Value(source)))
				}

				yamlString := strings.Join(contents, "")
				yamlDoc, err := yaml.Parse(yamlString)
				if err != nil {
					log.Info("failed to parse yaml from code block; its either not YAML or its invalid YAML", "yaml", yamlString, "err", err)
					continue
				}
				yDocs = append(yDocs, yamlDoc)
			}
		}

		for _, a := range n.Attributes() {
			fmt.Printf("Attribute: %v = %v\n", a.Name, a.Value)
		}
	}

	return yDocs, nil
}
