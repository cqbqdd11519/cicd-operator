package dispatcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	cicdv1 "github.com/tmax-cloud/cicd-operator/api/v1"
	"github.com/tmax-cloud/cicd-operator/internal/utils"
	"github.com/tmax-cloud/cicd-operator/pkg/git"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dispatcher dispatches IntegrationJob when webhook is called
// A kind of 'plugin' for webhook handler
type Dispatcher struct {
	Client client.Client
}

// Handle handles pull-request and push events
func (d Dispatcher) Handle(webhook *git.Webhook, config *cicdv1.IntegrationConfig) error {
	var job *cicdv1.IntegrationJob
	var err error
	pr := webhook.PullRequest
	push := webhook.Push
	if pr == nil && push == nil {
		return fmt.Errorf("pull request and push struct is nil")
	}

	if webhook.EventType == git.EventTypePullRequest && pr != nil {
		if pr.Action == git.PullRequestActionOpen || pr.Action == git.PullRequestActionSynchronize || pr.Action == git.PullRequestActionReOpen {
			job, err = GeneratePreSubmit(pr, &webhook.Repo, &pr.Sender, config)
			if err != nil {
				return err
			}
		}
	} else if webhook.EventType == git.EventTypePush && push != nil {
		job, err = GeneratePostSubmit(push, &webhook.Repo, &push.Sender, config)
		if err != nil {
			return err
		}
	}

	if job == nil {
		return nil
	}

	if err := d.Client.Create(context.Background(), job); err != nil {
		return err
	}

	return nil
}

// GeneratePreSubmit generates IntegrationJob for pull request event
func GeneratePreSubmit(pr *git.PullRequest, repo *git.Repository, sender *git.User, config *cicdv1.IntegrationConfig) (*cicdv1.IntegrationJob, error) {
	jobs, err := filter(config.Spec.Jobs.PreSubmit, git.EventTypePullRequest, pr.Base.Ref)
	if err != nil {
		return nil, err
	}
	if len(jobs) < 1 {
		return nil, nil
	}
	jobID := utils.RandomString(20)
	return &cicdv1.IntegrationJob{
		ObjectMeta: generateMeta(config.Name, config.Namespace, pr.Head.Sha, jobID),
		Spec: cicdv1.IntegrationJobSpec{
			ConfigRef: cicdv1.IntegrationJobConfigRef{
				Name: config.Name,
				Type: cicdv1.JobTypePreSubmit,
			},
			ID:         jobID,
			Jobs:       jobs,
			Workspaces: config.Spec.Workspaces,
			Refs: cicdv1.IntegrationJobRefs{
				Repository: repo.Name,
				Link:       repo.URL,
				Sender: &cicdv1.IntegrationJobSender{
					Name:  sender.Name,
					Email: sender.Email,
				},
				Base: cicdv1.IntegrationJobRefsBase{
					Ref:  pr.Base.Ref,
					Link: repo.URL,
				},
				Pull: &cicdv1.IntegrationJobRefsPull{
					ID:   pr.ID,
					Ref:  pr.Head.Ref,
					Sha:  pr.Head.Sha,
					Link: pr.URL,
					Author: cicdv1.IntegrationJobRefsPullAuthor{
						Name: pr.Sender.Name,
					},
				},
			},
			PodTemplate: config.Spec.PodTemplate,
		},
	}, nil
}

// GeneratePostSubmit generates IntegrationJob for push event
func GeneratePostSubmit(push *git.Push, repo *git.Repository, sender *git.User, config *cicdv1.IntegrationConfig) (*cicdv1.IntegrationJob, error) {
	jobs, err := filter(config.Spec.Jobs.PostSubmit, git.EventTypePush, push.Ref)
	if err != nil {
		return nil, err
	}
	if len(jobs) < 1 {
		return nil, nil
	}
	jobID := utils.RandomString(20)
	return &cicdv1.IntegrationJob{
		ObjectMeta: generateMeta(config.Name, config.Namespace, push.Sha, jobID),
		Spec: cicdv1.IntegrationJobSpec{
			ConfigRef: cicdv1.IntegrationJobConfigRef{
				Name: config.Name,
				Type: cicdv1.JobTypePostSubmit,
			},
			ID:         jobID,
			Jobs:       jobs,
			Workspaces: config.Spec.Workspaces,
			Refs: cicdv1.IntegrationJobRefs{
				Repository: repo.Name,
				Link:       repo.URL,
				Sender: &cicdv1.IntegrationJobSender{
					Name:  sender.Name,
					Email: sender.Email,
				},
				Base: cicdv1.IntegrationJobRefsBase{
					Ref:  push.Ref,
					Link: repo.URL,
					Sha:  push.Sha,
				},
			},
			PodTemplate: config.Spec.PodTemplate,
		},
	}, nil
}

func generateMeta(cfgName, cfgNamespace, sha, jobID string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-%s-%s", cfgName, sha[:5], jobID[:5]),
		Namespace: cfgNamespace,
		Labels: map[string]string{
			cicdv1.JobLabelConfig: cfgName,
			cicdv1.JobLabelID:     jobID,
		},
	}
}

func filter(cand []cicdv1.Job, evType git.EventType, ref string) ([]cicdv1.Job, error) {
	var filteredJobs []cicdv1.Job
	var incomingBranch string
	var incomingTag string

	switch evType {
	case git.EventTypePullRequest:
		incomingBranch = ref
	case git.EventTypePush:
		if strings.Contains(ref, "refs/tags/") {
			incomingTag = strings.Replace(ref, "refs/tags/", "", -1)
		} else {
			incomingBranch = strings.Replace(ref, "refs/heads/", "", -1)
		}
	}

	//tag push events
	var err error
	filteredJobs, err = filterTags(cand, incomingTag)
	if err != nil {
		return nil, err
	}
	filteredJobs, err = filterBranches(filteredJobs, incomingBranch)
	if err != nil {
		return nil, err
	}
	return filteredJobs, nil
}

func filterTags(jobs []cicdv1.Job, incomingTag string) ([]cicdv1.Job, error) {
	var filteredJobs []cicdv1.Job

	for _, job := range jobs {
		if job.When == nil {
			filteredJobs = append(filteredJobs, job)
			continue
		}
		tags := job.When.Tag
		skipTags := job.When.SkipTag

		// Always run if no tag/skipTag is specified
		if tags == nil && skipTags == nil {
			filteredJobs = append(filteredJobs, job)
		}

		if incomingTag == "" {
			continue
		}

		if tags != nil && skipTags == nil {
			for _, tag := range tags {
				if match := matchString(incomingTag, tag); match {
					filteredJobs = append(filteredJobs, job)
					break
				}
			}
		} else if skipTags != nil && tags == nil {
			isInValidJob := false
			for _, tag := range skipTags {
				if match := matchString(incomingTag, tag); match {
					isInValidJob = true
					break
				}
			}
			if !isInValidJob {
				filteredJobs = append(filteredJobs, job)
			}
		}
	}
	return filteredJobs, nil
}

func filterBranches(jobs []cicdv1.Job, incomingBranch string) ([]cicdv1.Job, error) {
	var filteredJobs []cicdv1.Job

	for _, job := range jobs {
		if job.When == nil {
			filteredJobs = append(filteredJobs, job)
			continue
		}
		branches := job.When.Branch
		skipBranches := job.When.SkipBranch

		// Always run if no branch/skipBranch is specified
		if branches == nil && skipBranches == nil {
			filteredJobs = append(filteredJobs, job)
		}

		if incomingBranch == "" {
			continue
		}

		if branches != nil && skipBranches == nil {
			for _, branch := range branches {
				if match := matchString(incomingBranch, branch); match {
					filteredJobs = append(filteredJobs, job)
					break
				}
			}
		} else if skipBranches != nil && branches == nil {
			isInValidJob := false
			for _, branch := range skipBranches {
				if match := matchString(incomingBranch, branch); match {
					isInValidJob = true
					break
				}
			}
			if !isInValidJob {
				filteredJobs = append(filteredJobs, job)
			}
		}
	}
	return filteredJobs, nil
}

func matchString(incoming, target string) bool {
	re, err := regexp.Compile(target)
	if err != nil {
		return false
	}
	return re.MatchString(incoming)
}
