package ecrutil

import (
	"encoding/json"
	"fmt"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"github.com/PrimerAI/hydros-public/pkg/util"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/aws/aws-sdk-go/aws/session"
)

// EcrPolicySyncController knows how to apply EcrPolicySync resources.
type EcrPolicySyncController struct {
	log  logr.Logger
	sess *session.Session
}

// NewEcrPolicySyncController create a new controller.
func NewEcrPolicySyncController(sess *session.Session, opts ...EcrPolicySyncOption) (*EcrPolicySyncController, error) {
	if sess == nil {
		return nil, fmt.Errorf("AWS session is required")
	}

	c := &EcrPolicySyncController{
		log:  zapr.NewLogger(zap.L()),
		sess: sess,
	}

	for _, o := range opts {
		o(c)
	}

	return c, nil
}

// EcrPolicySyncOption is an option
type EcrPolicySyncOption func(c *EcrPolicySyncController)

// EcrPolicySyncWithLogger create an option to use the logger.
func EcrPolicySyncWithLogger(log logr.Logger) EcrPolicySyncOption {
	return func(c *EcrPolicySyncController) {
		c.log = log
	}
}

// Apply applies the controller.
func (a *EcrPolicySyncController) Apply(n *yaml.RNode) error {
	r := &v1alpha1.EcrPolicySync{}

	log := a.log

	err := n.Document().Decode(r)
	if err != nil {
		log.Error(err, "Failed to decode EcrPolicySync")
		return err
	}

	log = a.log.WithValues("name", r.Metadata.Name)
	log.Info("Got EcrPolicySync", "EcrPolicySync", r)

	if r.Metadata.Name == "" {
		log.Info("Skipping EcrPolicySync;  Metadata.Name  must be set", "EcrPolicySync", r)
		return nil
	}

	svc := ecr.New(a.sess)

	expected := &map[string]interface{}{}
	if err := json.Unmarshal([]byte(r.Spec.Policy), expected); err != nil {
		log.Error(err, "Failed to unmarshal expected policy")
		return err
	}

	allErrors := &util.ListOfErrors{
		Causes: []error{},
	}

	for _, repo := range r.Spec.ImageRepos {
		log = log.WithValues("repo", repo)

		// Check the registry exists
		getIn := &ecr.GetRepositoryPolicyInput{
			RegistryId:     aws.String(r.Spec.ImageRegistry),
			RepositoryName: aws.String(repo),
		}
		currentPolicy, err := svc.GetRepositoryPolicy(getIn)
		if err != nil {
			aerr, ok := err.(awserr.Error)
			if !ok {
				log.Error(err, "GetRepositoryPolicy returned error that is not awserr.Error")
				allErrors.AddCause(errors.Wrapf(err, "Failed to get repo %v in registry %v", repo, r.Spec.ImageRegistry))
				continue
			}

			if aerr.Code() == "RepositoryNotFoundException" {
				log.Info("Repo doesn't exist")

				input := &ecr.CreateRepositoryInput{
					RepositoryName: aws.String(repo),
					// Add a tag to indicate it was created by hydros.
					Tags: []*ecr.Tag{
						{
							Key:   aws.String("createdby"),
							Value: aws.String("hydros"),
						},
					},
				}
				result, err := svc.CreateRepository(input)
				if err != nil {
					code := ""
					awsError := ""
					if aerr, ok := err.(awserr.Error); ok {
						code = aerr.Code()
						awsError = aerr.Error()
					}

					log.Error(err, "Failed to createRepo", "code", code, "awsError", awsError)
					allErrors.AddCause(errors.Wrapf(err, "Failed to create repo %v in registry %v", repo, r.Spec.ImageRegistry))
					continue
				}

				log.Info("Created repo", "output", result)
			} else if aerr.Code() == "RepositoryPolicyNotFoundException" {
				// Do nothing. This is expected because a repositor policy may not exist.
				// TODO(jeremy): If it does exist should we not override it.
			} else {
				log.Error(err, "Failed to fetch repo policy from ECR", "code", aerr.Code(), "awsError", aerr.Error())
				allErrors.AddCause(errors.Wrapf(err, "Failed to fetch repo policy for repo %v in registry %v", repo, r.Spec.ImageRegistry))
				continue
			}
		}

		if currentPolicy != nil {
			needsUpdate := func() bool {
				// To compare policies we deserialize the json to try to account for whitespace differences.
				actual := &map[string]interface{}{}

				if currentPolicy.PolicyText == nil {
					return true
				}

				if err := json.Unmarshal([]byte(*currentPolicy.PolicyText), actual); err != nil {
					log.Error(err, "Failed to unmarshal expected policy")
					return true
				}
				diff := cmp.Diff(expected, actual)
				if diff == "" {
					log.Info("Policy is up to date.", "currentPolicy", currentPolicy)
					return false
				}
				log.Info("Policy needs updating", "diff", diff)
				return true
			}()
			if !needsUpdate {
				continue
			}
		}
		input := &ecr.SetRepositoryPolicyInput{
			PolicyText:     aws.String(r.Spec.Policy),
			RegistryId:     aws.String(r.Spec.ImageRegistry),
			RepositoryName: aws.String(repo),
		}
		output, err := svc.SetRepositoryPolicy(input)
		if err != nil {
			log.Error(err, "Failed to set repository policy.", "output", output)
			allErrors.AddCause(errors.Wrapf(err, "Failed to update repo %v in registry %v", repo, r.Spec.ImageRegistry))
			continue
		}

		log.Info("Set repository policy succeeded", "output", output)
	}

	if len(allErrors.Causes) == 0 {
		return nil
	}
	allErrors.Final = fmt.Errorf("failed to update one or more repos")
	return allErrors
}

// GroupVersionKinds return GVKs.
func (a *EcrPolicySyncController) GroupVersionKinds() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		v1alpha1.EcrPolicySyncGVK,
	}
}
