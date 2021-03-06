package github

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PullRequestWebhook is a github-specific pull-request event webhook body
type PullRequestWebhook struct {
	Action string `json:"action"`
	Number int    `json:"number"`
	Sender User   `json:"sender"`

	PullRequest PullRequest `json:"pull_request"`

	Repo Repo `json:"repository"`
}

// PushWebhook is a github-specific push event webhook body
type PushWebhook struct {
	Ref    string `json:"ref"`
	Repo   Repo   `json:"repository"`
	Sender User   `json:"sender"`
	Sha    string `json:"after"`
}

// IssueCommentWebhook is a github-specific issue_comment webhook body
type IssueCommentWebhook struct {
	Action  string  `json:"action"`
	Comment Comment `json:"comment"`
	Issue   struct {
		PullRequest struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	} `json:"issue"`
	Repo   Repo `json:"repository"`
	Sender User `json:"sender"`
}

// PullRequestReviewWebhook is a github-specific pull_request_review webhook body
type PullRequestReviewWebhook struct {
	Action string `json:"action"`
	Review struct {
		Body        string       `json:"body"`
		SubmittedAt *metav1.Time `json:"submitted_at"`
	} `json:"review"`
	PullRequest PullRequest `json:"pull_request"`
	Repo        Repo        `json:"repository"`
	Sender      User        `json:"sender"`
}

// PullRequestReviewCommentWebhook is a github-specific pull_request_review_comment webhook body
type PullRequestReviewCommentWebhook struct {
	Action      string      `json:"action"`
	Comment     Comment     `json:"comment"`
	PullRequest PullRequest `json:"pull_request"`
	Repo        Repo        `json:"repository"`
	Sender      User        `json:"sender"`
}

// Repo structure for webhook event
type Repo struct {
	Name  string `json:"full_name"`
	URL   string `json:"html_url"`
	Owner struct {
		ID string `json:"login"`
	} `json:"owner"`
	Private bool `json:"private"`
}

// PullRequest is a pull request info
type PullRequest struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
	State  string `json:"state"`
	URL    string `json:"html_url"`
	User   User   `json:"user"`
	Head   struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// User is a sender of the event
type User struct {
	Name string `json:"login"`
	ID   int    `json:"id"`
}

// Comment is a comment payload
type Comment struct {
	Body      string       `json:"body"`
	CreatedAt *metav1.Time `json:"created_at"`
	UpdatedAt *metav1.Time `json:"updated_at"`
}

// RegistrationWebhookBody is a request body for registering webhook to remote git server
type RegistrationWebhookBody struct {
	Name   string                        `json:"name"`
	Active bool                          `json:"active"`
	Events []string                      `json:"events"`
	Config RegistrationWebhookBodyConfig `json:"config"`
}

// RegistrationWebhookBodyConfig is a config for the webhook
type RegistrationWebhookBodyConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	InsecureSsl string `json:"insecure_ssl"`
	Secret      string `json:"secret"`
}

// WebhookEntry is a body of list of registered webhooks
type WebhookEntry struct {
	ID     int `json:"id"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}
