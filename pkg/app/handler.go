package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-logr/zapr"
	"github.com/google/go-github/v52/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/githubapp"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type HydrosHandler struct {
	githubapp.ClientCreator
}

func (h *HydrosHandler) Handles() []string {
	return []string{"push"}
}

func (h *HydrosHandler) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	// TODO(jeremy): How do we distinguish between pushing a PR and merging a PR.
	log := zapr.NewLogger(zap.L()).WithValues("eventType", eventType, "deliverID", deliveryID)
	log.V(util.Debug).Info("Got github webhook")
	r := bytes.NewBuffer(payload)
	d := json.NewDecoder(r)
	event := &github.PushEvent{}
	if err := d.Decode(event); err != nil {
		log.Error(err, "Failed to decode a PushEvent")
		return err
	}

	//var event github.IssueCommentEvent
	//if err := json.Unmarshal(payload, &event); err != nil {
	//	return errors.Wrap(err, "failed to parse issue comment event payload")
	//}
	//
	//if !event.GetIssue().IsPullRequest() {
	//	zerolog.Ctx(ctx).Debug().Msg("Issue comment event is not for a pull request")
	//	return nil
	//}

	repo := event.GetRepo()
	action := event.GetAction()
	log.Info("Got push event", "url", repo.GetURL(), "action", action)

	// TODO(jeremy): Is this something we want to do? Its what the palantir sample app does
	// ctx, logger := githubapp.PreparePRContext(ctx, installationID, repo, event.GetIssue().GetNumber())

	//logger.Debug().Msgf("Event action is %s", event.GetAction())
	//if event.GetAction() != "created" {
	//	return nil
	//}

	repoName, err := ghrepo.FromFullName(repo.GetFullName())
	if err != nil {
		return err
	}
	//repoOwner := repo.GetOwner().GetLogin()
	//repoName := repo.GetName()
	//author := event.GetComment().GetUser().GetLogin()
	//body := event.GetComment().GetBody()
	//
	//if strings.HasSuffix(author, "[bot]") {
	//	logger.Debug().Msg("Issue comment was created by a bot")
	//	return nil
	//}
	//
	//logger.Debug().Msgf("Echoing comment on %s/%s#%d by %s", repoOwner, repoName, prNum, author)
	//msg := fmt.Sprintf("%s\n%s said\n```\n%s\n```\n", h.preamble, author, body)
	//prComment := github.IssueComment{
	//	Body: &msg,
	//}
	//
	//if _, _, err := client.Issues.CreateComment(ctx, repoOwner, repoName, prNum, &prComment); err != nil {
	//	logger.Error().Err(err).Msg("Failed to comment on pull request")
	//}

	// TODO(jeremy): We can use the checks client to create checks.
	installationID := githubapp.GetInstallationIDFromEvent(event)
	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	// TODO(jeremy): Payload in push_event contains a list of added/removed/modified files. We could use that
	// to determine whether hydros needs to run.

	// Check if its the main branch. And if not we don't run AI generation.

	if event.GetRef() != "refs/heads/main" {
		log.Info("Not main branch. Skipping", "ref", event.GetRef())

		// Update the PR with a CreateCheckRun

		check, response, err := client.Checks.CreateCheckRun(ctx, repoName.RepoOwner(), repoName.RepoName(), github.CreateCheckRunOptions{
			Name:       "hydros",
			HeadSHA:    event.GetAfter(),
			DetailsURL: proto.String("https://url.not.set.yet"),
			Status:     proto.String("completed"),
			Conclusion: proto.String("skipped"),
			Output: &github.CheckRunOutput{
				Title:   proto.String("Hydros skipped"),
				Summary: proto.String("Hydros skipped"),
				Text:    proto.String("Hydros skipped because this is not the main branch."),
			},
		})

		//client.Checks.UpdateCheckRun(ctx, repoName.RepoOwner(), repoName.RepoName(), check.GetID(), github.UpdateCheckRunOptions{
		//
		if err != nil {
			log.Error(err, "Failed to create check")
			return err
		}

		log.Info("Created check", "check", check, "response", response)
		return nil
	}
	//ref := event.GetRef()

	// https://docs.github.com/en/webhooks-and-events/webhooks/webhook-events-and-payloads#push
	// I think "after" is the commit after the push.
	return nil
}
