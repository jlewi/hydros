package skaffold

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/kustomize"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sLabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// File struct represents the skaffold file
type File struct {
	Path   string
	Config *SkaffoldConfig
}

// FilterPrefix returns a function that filters files that start with the given prefix.
func FilterPrefix(log logr.Logger, prefixes []string) kio.LocalPackageSkipFileFunc {
	return func(relPath string) bool {
		for _, prefix := range prefixes {
			if strings.HasPrefix(relPath, prefix) {
				log.V(util.Debug).Info("Skipping file", "path", relPath, "rule", prefix)
				return true
			}
		}
		return false
	}
}

// LoadSkaffoldConfigs loads all the skaffold configurations found in the specified directory
// that match the given LabelSelector.
func LoadSkaffoldConfigs(log logr.Logger, searchPath string, selector *v1alpha1.LabelSelector, excludes []string) ([]*File, error) {
	var configs []*File
	var s *k8sLabels.Selector

	if selector != nil {
		k8sSelector, err := selector.ToK8s()
		if err != nil {
			return configs, errors.Wrapf(err, "failed to convert label selector to K8s selector")
		}

		newS, err := meta.LabelSelectorAsSelector(k8sSelector)
		if err != nil {
			return configs, errors.Wrapf(err, "failed to invoke LabelSelectorAsSelector")
		}
		s = &newS
	}

	skipBadRead := kustomize.SkipBadRead(log, searchPath)
	filterPrefix := FilterPrefix(log, excludes)

	inputs := kio.LocalPackageReader{
		PackagePath:    searchPath,
		MatchFilesGlob: kio.MatchAll,
		FileSkipFunc: func(relPath string) bool {
			return filterPrefix(relPath) || skipBadRead(relPath)
		},
	}

	readers := []kio.Reader{inputs}

	// filter all the funcs
	findAllFn := kio.FilterFunc(func(operand []*yaml.RNode) ([]*yaml.RNode, error) {
		for i := range operand {
			resource := operand[i]
			if !strings.HasPrefix(resource.GetApiVersion(), "skaffold") {
				continue
			}

			// TODO(https://github.com/GoogleContainerTools/skaffold/pull/6782): Skaffold currently doesn't support
			// labels so the following code is commented out. We should enable if/when skaffold supports labels.
			if s != nil {
				labels := resource.GetLabels()
				if !(*s).Matches(k8sLabels.Set(labels)) {
					continue
				}
			}

			f, ok := resource.GetAnnotations()[kioutil.PathAnnotation]

			if !ok {
				// Try legacy annotation
				//nolint:staticcheck
				f, ok = resource.GetAnnotations()[kioutil.LegacyPathAnnotation]

				if !ok {
					return operand, fmt.Errorf("kio library didn't add the path annotation to the matching file")
				}
			}

			config := &SkaffoldConfig{}
			if err := resource.YNode().Decode(config); err != nil {
				return operand, errors.Wrapf(err, "could not decode file %v as SkaffoldConfig", f)
			}
			configs = append(configs, &File{
				Path:   path.Join(searchPath, f),
				Config: config,
			})
		}
		return operand, nil
	})

	err := kio.Pipeline{
		Inputs:  readers,
		Filters: []kio.Filter{findAllFn},
	}.Execute()

	return configs, err
}

// ChangeRegistry changes the registry of all images in the specified config to the supplied registry
func ChangeRegistry(config *SkaffoldConfig, registry string) error {
	if registry == "" {
		return nil
	}

	for index, a := range config.Build.Artifacts {
		// Check if the image exists.
		image, err := util.ParseImageURL(a.ImageName)
		if err != nil {
			return errors.Wrapf(err, "failed to parse image; %v", a.ImageName)
		}

		image.Registry = registry

		config.Build.Artifacts[index].ImageName = image.ToURL()
	}
	return nil
}

// RunBuild runs skaffold build in the specified directory.
// skaffoldFile is the path to the skaffold file.
// buildDir is the directory from which to run skaffold. Defaults to the directory of skaffoldFile
func RunBuild(skaffoldFile string, buildDir string, tags []string, sess *session.Session, log logr.Logger) error {
	h := util.ExecHelper{
		Log: log,
	}

	dir, err := ioutil.TempDir("", "hydrosSkaffoldBuild")
	if err != nil {
		return errors.Wrapf(err, "Failed to create temporary directory for skaffold")
	}
	defer func() {
		err := os.RemoveAll(dir)
		if err != nil {
			log.Error(err, "Failed to cleanup dir in defer execution ", dir)
		}
	}()

	outFile := path.Join(dir, "image.json")
	// TODO(jeremy): Need to edit the skaffold.yaml file to set the registry and then write the skaffold
	// file.
	if buildDir == "" {
		buildDir = path.Dir(skaffoldFile)
	}

	cmd := exec.Command("skaffold", "build", "-f", skaffoldFile, "--file-output="+outFile)
	cmd.Dir = buildDir
	// Right now we just swallow the error. If the build fails the image won't exist and hydration
	// will fail and it will eventually get retried.
	log.Info("Running skaffold", "path", skaffoldFile)

	// TODO(jeremy): This doesn't stream output as the build happens; all the output is logged once when the command
	// finishes
	if err := h.Run(cmd); err != nil {
		log.Error(err, "skaffold build failed")
	}

	// TODO(jeremy): Really need to accumulate errors and check no errors occurred.
	data, err := ioutil.ReadFile(outFile)
	if err != nil {
		return errors.Wrapf(err, "Could not open skaffold output file; %v", outFile)
	}

	builds, err := ParseBuildOutput(data)
	if err != nil {
		return errors.Wrapf(err, "Failed to parse skaffold build output")
	}

	for _, b := range builds.Builds {
		log.Info("Tagging image", "image", b.ImageName, "tag", b.Tag, "tags", tags)
		err := ecrutil.AddTagsToImage(sess, b.Tag, tags)
		if err != nil {
			return errors.Wrapf(err, "Failed to add tags to image")
		}
	}

	return nil
}
