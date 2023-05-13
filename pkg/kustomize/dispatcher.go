package kustomize

import (
	"fmt"
	"github.com/jlewi/hydros/pkg/kustomize/fns/ai"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jlewi/hydros/pkg/kustomize/fns/patches"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/jlewi/hydros/pkg/kustomize/fns/configmap"
	"github.com/jlewi/hydros/pkg/kustomize/fns/envs"
	"github.com/jlewi/hydros/pkg/kustomize/fns/fields"
	"github.com/jlewi/hydros/pkg/kustomize/fns/images"
	"github.com/jlewi/hydros/pkg/kustomize/fns/labels"
	"github.com/jlewi/hydros/pkg/kustomize/fns/s3assets"
	"github.com/jlewi/hydros/pkg/util"
)

const (
	// SourceFunctionPath the source dir key where you can find a function in question
	SourceFunctionPath = "sourcefunctionpath"
	// FunctionTargetDir the target dir key for a function to be applied too
	FunctionTargetDir = "targetdir"
)

// Dispatcher dispatches to the matching API
type Dispatcher struct {
	Log logr.Logger
}

// dispatchTable maps configFunction Kinds to implementations
var dispatchTable = map[string]func() kio.Filter{
	ai.Kind:        ai.Filter,
	configmap.Kind: configmap.Filter,
	envs.Kind:      envs.Filter,
	fields.Kind:    fields.Filter,
	images.Kind:    images.Filter,
	labels.Kind:    labels.Filter,
	s3assets.Kind:  s3assets.Filter,
	patches.Kind:   patches.Filter,
}

func isValidFnKind(category string) bool {
	_, valid := dispatchTable[category]
	return valid
}

// SkipBadRead tries to read a file and returns true if there is an error,
// false otherwise. Used with kio.LocalPackageReader to skip yaml files that
// aren't parsable by kustomize.
// mostly copied from kio.readFile: https://github.com/kubernetes-sigs/kustomize/blob/a0c7997b6647d78a9b8f7c2f320bf7efb8256423/kyaml/kio/pkgio_reader.go#L258
func SkipBadRead(log logr.Logger, basePath string) kio.LocalPackageSkipFileFunc {
	return func(relPath string) bool {
		// Skip files that don't match any of the acceptable glob patterns
		isGlobMatch := false
		for _, globPattern := range kio.MatchAll {
			if match, err := filepath.Match(globPattern, filepath.Base(relPath)); err != nil {
				log.Error(err, "failed to call filepath.Match on relPath in SkipBadRead", "relPath", relPath, "globPattern", globPattern)
				return true
			} else if match {
				isGlobMatch = true
				break
			}
		}

		// if no glob patterns match, return true to skip this file
		if !isGlobMatch {
			return true
		}

		fullPath := path.Join(basePath, relPath)
		f, err := os.Open(fullPath)
		if err != nil {
			log.Error(err, "call to SkipBadRead failed to open file", "fullPath", fullPath)
			return true
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Error(err, "panic, failed to close file in SkipBadRead defer", "f", f)
				panic(err)
			}
		}()

		rr := &kio.ByteReader{
			DisableUnwrapping: true,
			Reader:            f,
		}
		_, err = rr.Read()
		if err != nil {
			log.V(util.Debug).Info("err from kio.ByteReader.Read in SkipBadRead", "fullPath", fullPath, "err", err)
		}

		// There isn't a great way to test specifically for parse errors because they just come from a bare errors.Errorf:
		// https://github.com/kubernetes-sigs/kustomize/blob/0889995a61de07ddd4e48d9969c003507345956d/kyaml/yaml/fns.go#L762
		return err != nil
	}
}

// RunOnDir dispatches the functionPath on the supplied sourceDirectory
func (d *Dispatcher) RunOnDir(sourceDir string, functionPaths []string) error {
	for _, p := range functionPaths {

		// reads all functions from the given path
		reader := kio.LocalPackageReader{PackagePath: p, MatchFilesGlob: kio.MatchAll, FileSkipFunc: SkipBadRead(d.Log, p)}
		configs, err := reader.Read()
		if err != nil {
			return errors.Wrapf(err, "Could not read functions from path %v", p)
		}

		// loads filters to extracted functions
		fns, err := d.loadFilters(configs)
		if err != nil {
			d.Log.Error(err, "hit unexpected error while trying to load filters for filtered func", "function_path", p, "target_dir", sourceDir)
			return err
		}

		// applies functions to dest target dir
		err = applyFunc(d.Log, fns, sourceDir)
		if err != nil {
			d.Log.Error(err, "hit unexpected error while trying to apply function", "function_path", p, "target_dir", sourceDir)
			return err
		}
	}
	return nil
}

// GetAllFuncs gets all functions from the supplied source directory
func (d *Dispatcher) GetAllFuncs(sourceDir []string) (kio.PackageBuffer, error) {
	var allFilteredFns []*yaml.RNode

	for _, funcDir := range sourceDir {
		inputs := kio.LocalPackageReader{
			PackagePath: funcDir, MatchFilesGlob: kio.MatchAll, FileSkipFunc: SkipBadRead(d.Log, funcDir),
		}
		readers := []kio.Reader{inputs}

		// filter all the funcs
		findAllFn := kio.FilterFunc(func(operand []*yaml.RNode) ([]*yaml.RNode, error) {
			for i := range operand {
				resource := operand[i]
				if isValidFnKind(resource.GetKind()) {
					allFilteredFns = append(allFilteredFns, resource)
				}
			}
			return operand, nil
		})

		err := kio.Pipeline{
			Inputs:  readers,
			Filters: []kio.Filter{findAllFn},
		}.Execute()
		if err != nil {
			return kio.PackageBuffer{Nodes: allFilteredFns}, err
		}

	}
	return kio.PackageBuffer{Nodes: allFilteredFns}, nil
}

// SortFns sorts functions so that functions with the longest paths come first
// copied from the kustomize library https://github.com/kubernetes-sigs/kustomize/blob/3ebdb3fcef66580417d18f44ac20572469e41fa5/kyaml/runfn/runfn.go#L337
func (d *Dispatcher) SortFns(buff kio.PackageBuffer) error {
	var outerErr error
	// sort the nodes so that we traverse them depth first
	// functions deeper in the file system tree should be run first
	sort.Slice(buff.Nodes, func(i, j int) bool {
		mi, _ := buff.Nodes[i].GetMeta()
		pi := filepath.ToSlash(mi.Annotations[kioutil.PathAnnotation])

		mj, _ := buff.Nodes[j].GetMeta()
		pj := filepath.ToSlash(mj.Annotations[kioutil.PathAnnotation])

		// If the path is the same, we decide the ordering based on the
		// index annotation.
		if pi == pj {
			iIndex, err := strconv.Atoi(mi.Annotations[kioutil.IndexAnnotation])
			if err != nil {
				outerErr = err
				return false
			}
			jIndex, err := strconv.Atoi(mj.Annotations[kioutil.IndexAnnotation])
			if err != nil {
				outerErr = err
				return false
			}
			return iIndex < jIndex
		}

		if filepath.Base(path.Dir(pi)) == "functions" {
			// don't count the functions dir, the functions are scoped 1 level above
			pi = filepath.Dir(path.Dir(pi))
		} else {
			pi = filepath.Dir(pi)
		}

		if filepath.Base(path.Dir(pj)) == "functions" {
			// don't count the functions dir, the functions are scoped 1 level above
			pj = filepath.Dir(path.Dir(pj))
		} else {
			pj = filepath.Dir(pj)
		}

		// i is "less" than j (comes earlier) if its depth is greater -- e.g. run
		// i before j if it is deeper in the directory structure
		li := len(strings.Split(pi, "/"))
		if pi == "." {
			// local dir should have 0 path elements instead of 1
			li = 0
		}
		lj := len(strings.Split(pj, "/"))
		if pj == "." {
			// local dir should have 0 path elements instead of 1
			lj = 0
		}
		if li != lj {
			// use greater-than because we want to sort with the longest
			// paths FIRST rather than last
			return li > lj
		}

		// sort by path names if depths are equal
		return pi < pj
	})
	return outerErr
}

// RemoveOverlayOnHydratedFiles will return a map of targetpaths that are separated by their overlay
func (d *Dispatcher) RemoveOverlayOnHydratedFiles(filesToHydrate []string, sourceRoot string) (map[TargetPath]bool, error) {
	targetPathSeperatedFilesToHydrate := map[TargetPath]bool{}
	for _, file := range filesToHydrate {
		targetpath, err := GenerateTargetPath(sourceRoot, file)
		if err != nil {
			d.Log.Error(err, "Unexpected error when trying to generate target path")
			return nil, err
		}
		targetPathSeperatedFilesToHydrate[targetpath] = true
	}
	return targetPathSeperatedFilesToHydrate, nil
}

func (d *Dispatcher) constructTargetPath(sourceRoot string, pathAnnotation string, leafPaths map[TargetPath]bool, hydratedPath string) (string, error) {
	// check if its a leaf or not, because we dont want to remove the overlay for a non overlay path
	// e.g. `a/b/dev/labels.yaml` -> `a/b` the target path
	// `a/b/c/labels.yaml` -> `a/b/c` the target path

	targetPath, err := GenerateTargetPath(sourceRoot, filepath.Join(sourceRoot, pathAnnotation))
	if err != nil {
		return "", err
	}
	if leafPaths[targetPath] {
		// if the path is a leaf, lets remove the overlay and return that path
		return filepath.Join(hydratedPath, targetPath.Dir), nil
	}
	// the path is not a leaf so we dont remove the dir
	return filepath.Join(hydratedPath, filepath.Dir(pathAnnotation)), nil
}

// SetFuncPaths function adds 2 annotations; sourcefunctionpath that will be the source of where the functionpath lies;
// targetdir is the target directory we need to apply the function too
func (d *Dispatcher) SetFuncPaths(buff kio.PackageBuffer, hydratedPath string, sourceRoot string, leafPaths map[TargetPath]bool) error {
	for _, filteredFunc := range buff.Nodes {

		annotations := filteredFunc.GetAnnotations()
		var pathAnnotation string

		// applied this change because we dont know which one we are using
		// ref: https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml/kio/kioutil@v0.12.0#PathAnnotation
		if pathAnnon, ok := annotations[kioutil.PathAnnotation]; ok {
			pathAnnotation = pathAnnon
		} else {
			//nolint:staticcheck
			pathAnnotation = annotations[kioutil.LegacyPathAnnotation]
		}

		targetDir, err := d.constructTargetPath(sourceRoot, pathAnnotation, leafPaths, hydratedPath)
		if err != nil {
			return err
		}

		annotations[SourceFunctionPath] = filepath.Dir(filepath.Join(sourceRoot, pathAnnotation))
		annotations[FunctionTargetDir] = targetDir
		err = filteredFunc.SetAnnotations(annotations)
		if err != nil {
			return err
		}
	}
	return nil
}

// loadFilters
func (d *Dispatcher) loadFilters(configs []*yaml.RNode) ([]kio.Filter, error) {
	log := d.Log

	tempFns := []kio.Filter{}
	tempCmFns := []kio.Filter{}

	for _, n := range configs {
		m, err := n.GetMeta()
		if err != nil {
			log.Info("Skipping YAML node missing meta")
			continue
		}

		fn, ok := dispatchTable[m.Kind]
		if !ok {
			log.Info("Skipping kind; not a fn", "kind", m.Kind, "name", m.Name)
			continue
		}
		log := log.WithValues("kind", m.Kind, "name", m.Name, "config_path", m.Annotations[kioutil.PathAnnotation])
		fltr := fn()

		err = n.YNode().Decode(fltr)

		if err != nil {
			log.Error(err, "Failed to decode fn")
			return nil, err
		}

		if val, ok := m.Annotations[configmap.ConfigMapAnnotation]; ok {
			val = strings.TrimSpace(strings.ToLower(val))

			if val == configmap.ConfigMapBoth {
				log.Info("Adding kustomize function", "kind", m.Kind)
				tempFns = append(tempFns, fltr)
			}

			log.Info("Adding configmap wrapped kustomize function", "kind", m.Kind)
			tempCmFns = append(tempCmFns, fltr)
		} else {
			log.Info("Adding kustomize function", "kind", m.Kind)
			tempFns = append(tempFns, fltr)
		}
	}

	fns := []kio.Filter{}
	cmFns := configmap.WrappedFilter{
		Filters: tempCmFns,
	}

	fns = append(fns, tempFns...)

	if len(cmFns.Filters) > 0 {
		log.Info("Adding filters for configmaps", "number", len(cmFns.Filters))
		fns = append(fns, cmFns)
	}

	fns = append(fns, &filters.MergeFilter{}, &filters.FileSetter{FilenamePattern: filepath.Join("config", "%n.yaml")},
		&filters.FormatFilter{},
	)

	return fns, nil
}

// ApplyFilteredFuncs will apply all the functions to its specific directory, as
func (d *Dispatcher) ApplyFilteredFuncs(filteredFuncs []*yaml.RNode) error {
	// loop that will go though a list of all filtered functions in a repo and modify the annotation according
	// to the dir they are part of
	for _, filteredFunc := range filteredFuncs {
		var targetdir string
		annotations := filteredFunc.GetAnnotations()

		if val, ok := annotations[FunctionTargetDir]; ok {
			targetdir = val
		} else {
			err := fmt.Errorf("functiontargetdir is empty, for Func: %v", filteredFunc)
			d.Log.Error(err, "hit unexpected error while trying to apply functions")
			return err
		}
		if _, err := os.Stat(targetdir); os.IsNotExist(err) {
			// target path Dir does not exist so we cannot apply function
			d.Log.Info("target Dir to apply function to does not exist, skipping execution",
				"function", filteredFunc, "targetdir", targetdir)
			continue
		}

		fns, err := d.loadFilters([]*yaml.RNode{filteredFunc})
		if err != nil {
			d.Log.Error(err, "hit unexpected error while trying to append Function and ConfigMap filters", "function", annotations[kioutil.PathAnnotation])
			return err
		}

		err = applyFunc(d.Log, fns, targetdir)
		if err != nil {
			d.Log.Error(err, "hit unexpected error while trying to apply function", "function", annotations[kioutil.PathAnnotation])
			return err
		}
	}
	return nil
}

// applyFunc, applies a set of fns to a specified directory
func applyFunc(log logr.Logger, fns []kio.Filter, targetDir string) error {
	inputs := kio.LocalPackageReader{
		PackagePath:    targetDir,
		MatchFilesGlob: kio.MatchAll,
		FileSkipFunc:   SkipBadRead(log, targetDir),
	}

	w := kio.LocalPackageWriter{
		PackagePath:           targetDir,
		KeepReaderAnnotations: false,
	}

	return kio.Pipeline{
		Inputs:  []kio.Reader{inputs},
		Filters: fns,
		Outputs: []kio.Writer{w},
	}.Execute()
}
