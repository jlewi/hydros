package github

import "fmt"

// PullRequest is a struct for representing PRs. This was largely copied from GitHub's CLI.
type PullRequest struct {
	ID          string
	Number      int
	Title       string
	State       string
	Closed      bool
	URL         string
	BaseRefName string
	HeadRefName string
	Body        string
	Mergeable   string

	Author struct {
		Login string
	}
	HeadRepositoryOwner struct {
		Login string
	}
	HeadRepository struct {
		Name             string
		DefaultBranchRef struct {
			Name string
		}
	}
	IsCrossRepository   bool
	IsDraft             bool
	MaintainerCanModify bool

	ReviewDecision string

	Commits struct {
		TotalCount int
		Nodes      []struct {
			Commit struct {
				StatusCheckRollup struct {
					Contexts struct {
						Nodes []struct {
							State      string
							Status     string
							Conclusion string
						}
					}
				}
			}
		}
	}
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				TypeName string `json:"__typename"`
				Login    string
				Name     string
			}
		}
		TotalCount int
	}
	Reviews struct {
		Nodes []struct {
			Author struct {
				Login string
			}
			State string
		}
	}
	Assignees struct {
		Nodes []struct {
			Login string
		}
		TotalCount int
	}
	Labels struct {
		Nodes []struct {
			Name string
		}
		TotalCount int
	}
	ProjectCards struct {
		Nodes []struct {
			Project struct {
				Name string
			}
			Column struct {
				Name string
			}
		}
		TotalCount int
	}
	Milestone struct {
		Title string
	}
}

// HeadLabel returns the label for the head reference.
func (pr PullRequest) HeadLabel() string {
	if pr.IsCrossRepository {
		return fmt.Sprintf("%s:%s", pr.HeadRepositoryOwner.Login, pr.HeadRefName)
	}
	return pr.HeadRefName
}
