package ai

import (
	"fmt"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"strings"
	"text/template"
)

const (
	AnnotationPrefix = "ai.hydros.io/"

	// TODO(jeremy): Augment this with openAPI definitions for extra APIs
	systemContextTmpl = `You are a Kubernetes and Google Cloud expert. Your speciality is in generating
YAML definitions of resources using the Kubernetes Resource Model. Your job is to translate natural language
descriptions of infrastructure into the corresponding YAML definitions. In addition to the built in resources
you can also use custom resources. Included below is a list of openapi schemas for custom resources. Each
schema defines a single resource and is encoded in JSON one per line.

--- Begin schemas ---
{{range .Schemas}}{{.}}{{end}}
--- End schemas ---
`
)

// Generator handles extracting annotations from Kubernetes resources and generating configuration.
type Generator struct {
}

// buildPrompt builds the system context prompt given a list of openAPI schemas.
func buildPrompt(apiSpecs []*yaml.RNode) (string, error) {
	t, err := template.New("prompt").Parse(systemContextTmpl)
	if err != nil {
		return "", err
	}

	data := struct {
		Schemas []string
	}{
		Schemas: make([]string, 0, len(apiSpecs)),
	}
	for _, n := range apiSpecs {
		b, err := n.MarshalJSON()
		if err != nil {
			return "", errors.Wrapf(err, "Failed to marshal openapi spec to JSON")
		}
		data.Schemas = append(data.Schemas, string(b))
	}

	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", errors.Wrapf(err, "Failed to build the prompt")
	}

	return sb.String(), nil
}

// Process handles all the resources in a directory.
func (g *Generator) Process(dir string) error {
	files, err := util.FindYamlFiles(dir)
	if err != nil {
		return errors.Wrapf(err, "Error finding YAML files in %v", dir)
	}

	for _, file := range files {
		nodes, err := util.ReadYaml(file)
		if err != nil {
			return errors.Wrapf(err, "Error reading YAML file %v", file)
		}
		for _, node := range nodes {
			annotations := node.GetAnnotations()
			if annotations == nil {
				continue
			}
			for k, v := range annotations {
				if !strings.HasPrefix(k, AnnotationPrefix) {
					continue
				}

				// Handle the annotation by sending it to the model.
				fmt.Printf("%v", v)
			}
		}
	}
	return nil
}
