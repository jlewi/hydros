package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/PullRequestInc/go-gpt3"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns/ai/openai"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"path/filepath"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"strconv"
	"strings"
	"text/template"
	"time"
)

const (
	OwnerPrefix      = "owner.hydros.io/"
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

const (
	// Kind is the kind for the kustomize function.
	Kind = "HydrosAI"
)

var _ kio.Filter = &GeneratorFn{}

// Filter returns a new CommonLabelsFn
func Filter() kio.Filter {
	return &GeneratorFn{}
}

// GeneratorFn handles extracting annotations from Kubernetes resources and generating configuration.
type GeneratorFn struct {
	// Kind is the API name.  Must be PodEnvs.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata v1alpha1.Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`

	// completer to use
	completer Completer
	client    gpt3.Client
	// system is the system prompt to use
	system string
}

// Spec is the spec for the GeneratorFn function.
type Spec struct {
	// FilterSpecs is a list of strings providing the openapi specs of the functions that can be invoked.
	// TODO(jeremy): Should we use a CRD here? That way we would know the kind etc...
	FilterSpecs []map[string]interface{} `json:"filterSpecs,omitempty" yaml:"filterSpecs,omitempty"`
}

// TODO(jeremy): If we wrap the functions in CRD spec then we could potentially just load them from the directory
// like we do with KPT functions
func buildPromptFromDirs(dirs []string) (string, error) {
	log := zapr.NewLogger(zap.L())
	specs := make([]*yaml.RNode, 0, 10)
	for _, d := range dirs {
		files, err := util.FindYamlFiles(d)
		if err != nil {
			return "", errors.Wrapf(err, "Error finding YAML files in %v", d)
		}
		for _, f := range files {
			nodes, err := util.ReadYaml(f)
			if err != nil {
				return "", errors.Wrapf(err, "Error reading YAML files in %v", d)
			}

			for _, n := range nodes {
				v := n.Field("openapi")
				if v == nil {
					log.V(util.Debug).Info("skipping node which doesn't contain openapi spec", "file", f)
					continue
				}
				specs = append(specs, n)
			}
		}
	}

	return buildPrompt(specs)
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

type PromptSource struct {
	Node   *yaml.RNode
	File   string
	Prompt string
	// Key is the annotation key
	Key string
}

type PromptRef struct {
	// The hash of the prompt that generated this
	Hash string `json:"hash" yaml:"hash"`
	// We want to store the prompt so we have good labeled data and also.
	Prompt string `json:"prompt" yaml:"prompt"`

	// Raw response from openai
	// N.B. useful for debugging and development not sure if we actually want to store the hole response
	Response string `json:"response" yaml:"response"`
}

func hashPrompt(prompt string) string {
	h := sha256.New()
	h.Write([]byte(prompt))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (g *GeneratorFn) init() error {
	apiKey := openai.GetAPIKey()
	if apiKey == "" {
		return errors.New("No OpenAI API key specified. Set the environment variable OPENAI_API_KEY.")
	}

	g.client = gpt3.NewClient(string(apiKey), gpt3.WithTimeout(1*time.Minute))

	specs := make([]*yaml.RNode, 0, len(g.Spec.FilterSpecs))
	for _, s := range g.Spec.FilterSpecs {
		n, err := yaml.FromMap(s)
		if err != nil {
			return errors.Wrapf(err, "Failed to marshal the spec")
		}
		specs = append(specs, n)
	}

	systemPrompt, err := buildPrompt(specs)
	if err != nil {
		return errors.Wrapf(err, "Failed to build the system prompt from list of known APISpecs")
	}
	g.system = systemPrompt

	g.completer = g.Complete
	return nil
}

// Filter looks for any relevant annotations providing prompts and inflates them if necessary.
func (g *GeneratorFn) Filter(in []*yaml.RNode) ([]*yaml.RNode, error) {
	log := zapr.NewLogger(zap.L())

	if err := g.init(); err != nil {
		return in, errors.Wrapf(err, "Failed to initialize GeneratorFn")
	}

	// Build a map of the prompts that have already been generated.
	prompts := make(map[string]bool)

	// List of prompts and the associated files; node from which they came.
	sources := make([]*PromptSource, 0, 10)

	for _, node := range in {
		annotations := node.GetAnnotations()
		if annotations == nil {
			continue
		}
		for k, v := range annotations {
			if strings.HasPrefix(k, OwnerPrefix) {
				// The owner prefix annotation is used to contain information about which prompt generated
				// this resource if any.
				ref := &PromptRef{}
				if err := json.Unmarshal([]byte(v), ref); err != nil {
					log.Error(err, "Failed to unmarshal owner annotation", "key", k, "value", v)
					continue
				}

				prompts[ref.Hash] = true
			}
			file := annotations[kioutil.PathAnnotation]
			if strings.HasPrefix(k, AnnotationPrefix) {
				p := &PromptSource{
					Node:   node,
					File:   file,
					Prompt: v,
					Key:    k,
				}

				sources = append(sources, p)
			}
		}
	}

	out := in
	failures := &util.ListOfErrors{}

	// Process unprocessed prompts
	for _, p := range sources {
		hash := hashPrompt(p.Prompt)

		if _, ok := prompts[hash]; ok {
			log.Info("Prompt already processed", "prompt", p.Prompt, "file", p.File, "key", p.Key)
			continue
		}

		log.Info("Processing prompt", "prompt", p.Prompt, "file", p.File, "key", p.Key)
		resp, extra, err := g.completer(p.Prompt)
		if err != nil {
			log.Error(err, "Failed to complete prompt", "prompt", p.Prompt, "file", p.File, "key", p.Key)
			failures.AddCause(errors.Wrapf(err, "Failed to complete prompt %v", p.Prompt))
			continue
		}

		ref := PromptRef{
			Hash:     hash,
			Prompt:   p.Prompt,
			Response: extra,
		}

		b, err := json.Marshal(ref)
		if err != nil {
			log.Error(err, "Failed to marshal prompt ref", "prompt", p.Prompt, "file", p.File, "key", p.Key)
			failures.AddCause(errors.Wrapf(err, "Failed to marshal prompt ref %v", ref))
			continue
		}

		// Get filepath without the extension
		name := strings.TrimSuffix(p.File, filepath.Ext(p.File))
		name += "_ai_generated.yaml"
		for i, n := range resp {
			annotations := n.GetAnnotations()
			annotations[kioutil.PathAnnotation] = name
			annotations[kioutil.IndexAnnotation] = strconv.Itoa(i)
			annotations[OwnerPrefix] = string(b)
			n.SetAnnotations(annotations)
		}

		// Add all the newly generated resources to the list of resources
		out = append(out, resp...)
	}

	return out, nil
}

func (g *GeneratorFn) Complete(prompt string) ([]*yaml.RNode, string, error) {
	log := zapr.NewLogger(zap.L())
	req := gpt3.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []gpt3.ChatCompletionRequestMessage{
			{
				Role:    "system",
				Content: g.system,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	resp, err := g.client.ChatCompletion(context.Background(), req)
	if err != nil {
		return nil, "", errors.Wrapf(err, "Failed to complete prompt %v", prompt)
	}
	if len(resp.Choices) > 1 {
		log.Info("Warning multiple choices returned only the first one is being used", "prompt", prompt, "choices", resp.Choices)
	}

	nodes, err := MarkdownToYAML(resp.Choices[0].Message.Content)

	raw, err := json.Marshal(resp)
	if err != nil {
		return nodes, "", errors.Wrapf(err, "Failed to marshal response %v", resp)
	}
	return nodes, string(raw), nil
}

//// Process handles all the resources in a directory.
//// TODO(jeremy): Can we get rid of this function?
//func (g *GeneratorFn) Process(dir string) error {
//	log := zapr.NewLogger(zap.L())
//	files, err := util.FindYamlFiles(dir)
//	if err != nil {
//		return errors.Wrapf(err, "Error finding YAML files in %v", dir)
//	}
//
//	// Build a map of the prompts that have already been generated.
//	prompts := make(map[string]bool)
//
//	// List of prompts and the associated files; node from which they came.
//	sources := make([]*PromptSource, 0, 10)
//
//	for _, file := range files {
//		nodes, err := util.ReadYaml(file)
//		if err != nil {
//			return errors.Wrapf(err, "Error reading YAML file %v", file)
//		}
//		for _, node := range nodes {
//			annotations := node.GetAnnotations()
//			if annotations == nil {
//				continue
//			}
//			for k, v := range annotations {
//				if strings.HasPrefix(k, OwnerPrefix) {
//					// The owner prefix annotation is used to contain information about which prompt generated
//					// this resource if any.
//					ref := &PromptRef{}
//					if err := json.Unmarshal([]byte(v), ref); err != nil {
//						log.Error(err, "Failed to unmarshal owner annotation", "key", k, "value", v)
//						continue
//					}
//
//					prompts[ref.Hash] = true
//				}
//				if strings.HasPrefix(k, AnnotationPrefix) {
//					p := &PromptSource{
//						Node:   node,
//						File:   file,
//						Prompt: v,
//						Key:    k,
//					}
//
//					sources = append(sources, p)
//				}
//
//			}
//		}
//	}
//
//	return nil
//}

// Completer takes a prompt and returns YAML resource that contain the completion.
// Response is an empty list if no completions could be generated.
// The string is an extra metadata that should logged with the completion
type Completer func(prompt string) ([]*yaml.RNode, string, error)
