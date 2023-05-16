package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-logr/zapr"
	"github.com/google/go-github/v52/github"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/githubapp"
	"go.uber.org/zap"
)

type HydrosHandler struct {
	githubapp.ClientCreator

	preamble string
}

func (h *HydrosHandler) Handles() []string {
	return []string{"issue_comment"}
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
	//installationID := githubapp.GetInstallationIDFromEvent(event)
	//client, err := h.NewInstallationClient(installationID)
	//if err != nil {
	//	return err
	//}
	//client.Checks.CreateCheckRun(ctx, repo.GetOwner().GetName(), repo.GetName(), github.CreateCheckRunOptions{})

	return nil
}
