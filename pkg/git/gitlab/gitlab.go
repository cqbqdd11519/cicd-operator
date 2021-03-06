package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	cicdv1 "github.com/tmax-cloud/cicd-operator/api/v1"
	"github.com/tmax-cloud/cicd-operator/pkg/git"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is a gitlab client struct
type Client struct {
	IntegrationConfig *cicdv1.IntegrationConfig
	K8sClient         client.Client
}

// ParseWebhook parses a webhook body for gitlab
func (c *Client) ParseWebhook(header http.Header, jsonString []byte) (*git.Webhook, error) {
	if err := Validate(c.IntegrationConfig.Status.Secrets, header.Get("x-gitlab-token")); err != nil {
		return nil, err
	}

	eventFromHeader := header.Get("x-gitlab-event")
	switch eventFromHeader {
	case "Merge Request Hook":
		return c.parsePullRequestWebhook(jsonString)
	case "Push Hook", "Tag Push Hook":
		return c.parsePushWebhook(jsonString)
	case "Note Hook":
		return c.parseIssueComment(jsonString)
	}

	return nil, nil
}

// ListWebhook lists registered webhooks
func (c *Client) ListWebhook() ([]git.WebhookEntry, error) {
	encodedRepoPath := url.QueryEscape(c.IntegrationConfig.Spec.Git.Repository)
	apiURL := c.IntegrationConfig.Spec.Git.GetAPIUrl() + "/api/v4/projects/" + encodedRepoPath + "/hooks"

	data, _, err := c.requestHTTP(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	var entries []WebhookEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	var result []git.WebhookEntry
	for _, e := range entries {
		result = append(result, git.WebhookEntry{ID: e.ID, URL: e.URL})
	}

	return result, nil
}

// RegisterWebhook registers our webhook server to the remote git server
func (c *Client) RegisterWebhook(uri string) error {
	var registrationBody RegistrationWebhookBody
	EncodedRepoPath := url.QueryEscape(c.IntegrationConfig.Spec.Git.Repository)
	apiURL := c.IntegrationConfig.Spec.Git.GetAPIUrl() + "/api/v4/projects/" + EncodedRepoPath + "/hooks"

	//enable hooks from every events
	registrationBody.EnableSSLVerification = false
	registrationBody.ConfidentialIssueEvents = true
	registrationBody.ConfidentialNoteEvents = true
	registrationBody.DeploymentEvents = true
	registrationBody.IssueEvents = true
	registrationBody.JobEvents = true
	registrationBody.MergeRequestEvents = true
	registrationBody.NoteEvents = true
	registrationBody.PipeLineEvents = true
	registrationBody.PushEvents = true
	registrationBody.TagPushEvents = true
	registrationBody.WikiPageEvents = true
	registrationBody.URL = uri
	registrationBody.ID = EncodedRepoPath
	registrationBody.Token = c.IntegrationConfig.Status.Secrets

	if _, _, err := c.requestHTTP(http.MethodPost, apiURL, registrationBody); err != nil {
		return err
	}

	return nil
}

// DeleteWebhook deletes registered webhook
func (c *Client) DeleteWebhook(id int) error {
	encodedRepoPath := url.QueryEscape(c.IntegrationConfig.Spec.Git.Repository)
	apiURL := c.IntegrationConfig.Spec.Git.GetAPIUrl() + "/api/v4/projects/" + encodedRepoPath + "/hooks/" + strconv.Itoa(id)

	if _, _, err := c.requestHTTP(http.MethodDelete, apiURL, nil); err != nil {
		return err
	}

	return nil
}

// SetCommitStatus sets commit status for the specific commit
func (c *Client) SetCommitStatus(integrationJob *cicdv1.IntegrationJob, context string, state git.CommitStatusState, description, targetURL string) error {
	var commitStatusBody CommitStatusBody
	var urlEncodePath = url.QueryEscape(c.IntegrationConfig.Spec.Git.Repository)
	var sha string
	if integrationJob.Spec.Refs.Pull == nil {
		sha = integrationJob.Spec.Refs.Base.Sha
	} else {
		sha = integrationJob.Spec.Refs.Pull.Sha
	}
	apiURL := c.IntegrationConfig.Spec.Git.GetAPIUrl() + "/api/v4/projects/" + urlEncodePath + "/statuses/" + sha
	switch cicdv1.CommitStatusState(state) {
	case cicdv1.CommitStatusStatePending:
		commitStatusBody.State = "running"
	case cicdv1.CommitStatusStateFailure, cicdv1.CommitStatusStateError:
		commitStatusBody.State = "failed"
	default:
		commitStatusBody.State = string(state)
	}
	commitStatusBody.TargetURL = targetURL
	commitStatusBody.Description = description
	commitStatusBody.Context = context

	// Cannot transition status via :run from :running
	if _, _, err := c.requestHTTP(http.MethodPost, apiURL, commitStatusBody); err != nil && !strings.Contains(strings.ToLower(err.Error()), "cannot transition status via") {
		return err
	}

	return nil
}

// GetUserInfo gets a user's information
func (c *Client) GetUserInfo(userID string) (*git.User, error) {
	// userID is int!
	apiURL := fmt.Sprintf("%s/api/v4/users/%s", c.IntegrationConfig.Spec.Git.GetAPIUrl(), userID)

	result, _, err := c.requestHTTP(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	var userInfo UserInfo
	if err := json.Unmarshal(result, &userInfo); err != nil {
		return nil, err
	}

	email := userInfo.PublicEmail
	if email == "" {
		email = userInfo.Email
	}

	return &git.User{
		ID:    userInfo.ID,
		Name:  userInfo.UserName,
		Email: email,
	}, err
}

// CanUserWriteToRepo decides if the user has write permission on the repo
func (c *Client) CanUserWriteToRepo(user git.User) (bool, error) {
	// userID is int!
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/members/all/%d", c.IntegrationConfig.Spec.Git.GetAPIUrl(), url.QueryEscape(c.IntegrationConfig.Spec.Git.Repository), user.ID)

	result, _, err := c.requestHTTP(http.MethodGet, apiURL, nil)
	if err != nil {
		return false, err
	}

	var permission UserPermission
	if err := json.Unmarshal(result, &permission); err != nil {
		return false, err
	}

	return permission.AccessLevel >= 30, nil
}

// RegisterComment registers comment to an issue
func (c *Client) RegisterComment(issueType git.IssueType, issueNo int, body string) error {
	var t string
	switch issueType {
	case git.IssueTypeIssue:
		t = "issues"
	case git.IssueTypePullRequest:
		t = "merge_requests"
	default:
		return fmt.Errorf("issue type %s is not supported", issueType)
	}

	apiUrl := fmt.Sprintf("%s/api/v4/projects/%s/%s/%d/notes", c.IntegrationConfig.Spec.Git.GetAPIUrl(), url.QueryEscape(c.IntegrationConfig.Spec.Git.Repository), t, issueNo)

	commentBody := &CommentBody{Body: body}
	if _, _, err := c.requestHTTP(http.MethodPost, apiUrl, commentBody); err != nil {
		return err
	}
	return nil
}

func (c *Client) requestHTTP(method, apiURL string, data interface{}) ([]byte, http.Header, error) {
	token, err := c.IntegrationConfig.GetToken(c.K8sClient)
	if err != nil {
		return nil, nil, err
	}
	header := map[string]string{
		"PRIVATE-TOKEN": token,
		"Content-Type":  "application/json",
	}

	return git.RequestHTTP(method, apiURL, header, data)
}

// Validate validates the webhook payload
func Validate(secret, headerToken string) error {
	if secret != headerToken {
		return fmt.Errorf("invalid request : X-Gitlab-Token does not match secret")
	}
	return nil
}
