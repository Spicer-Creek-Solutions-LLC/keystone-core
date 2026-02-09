package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"text/template"
	"time"
)

// NotifierConfig contains configuration for approval notifications.
type NotifierConfig struct {
	// Slack configuration
	SlackWebhookURL string `yaml:"slackWebhookUrl" json:"slackWebhookUrl"`
	SlackChannel    string `yaml:"slackChannel" json:"slackChannel"`

	// Email configuration
	SMTPHost     string `yaml:"smtpHost" json:"smtpHost"`
	SMTPPort     int    `yaml:"smtpPort" json:"smtpPort"`
	SMTPUser     string `yaml:"smtpUser" json:"smtpUser"`
	SMTPPassword string `yaml:"smtpPassword" json:"smtpPassword"`
	EmailFrom    string `yaml:"emailFrom" json:"emailFrom"`

	// Approval URL base for action links
	ApprovalURLBase string `yaml:"approvalUrlBase" json:"approvalUrlBase"`

	// HTTP client timeout
	HTTPTimeout time.Duration `yaml:"httpTimeout" json:"httpTimeout"`
}

// DefaultNotifier provides multi-channel approval notifications.
type DefaultNotifier struct {
	config     NotifierConfig
	httpClient *http.Client

	// Templates
	slackRequestTemplate  *template.Template
	slackDecisionTemplate *template.Template
	slackReminderTemplate *template.Template
	slackExpiredTemplate  *template.Template

	emailRequestTemplate  *template.Template
	emailDecisionTemplate *template.Template
	emailReminderTemplate *template.Template
	emailExpiredTemplate  *template.Template
}

// NewDefaultNotifier creates a new notification handler.
func NewDefaultNotifier(config NotifierConfig) (*DefaultNotifier, error) {
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = 30 * time.Second
	}

	n := &DefaultNotifier{
		config: config,
		httpClient: &http.Client{
			Timeout: config.HTTPTimeout,
		},
	}

	if err := n.initTemplates(); err != nil {
		return nil, fmt.Errorf("init templates: %w", err)
	}

	return n, nil
}

// initTemplates initializes notification templates.
func (n *DefaultNotifier) initTemplates() error {
	var err error

	// Slack templates
	n.slackRequestTemplate, err = template.New("slack_request").Parse(slackRequestTpl)
	if err != nil {
		return fmt.Errorf("parse slack request template: %w", err)
	}

	n.slackDecisionTemplate, err = template.New("slack_decision").Parse(slackDecisionTpl)
	if err != nil {
		return fmt.Errorf("parse slack decision template: %w", err)
	}

	n.slackReminderTemplate, err = template.New("slack_reminder").Parse(slackReminderTpl)
	if err != nil {
		return fmt.Errorf("parse slack reminder template: %w", err)
	}

	n.slackExpiredTemplate, err = template.New("slack_expired").Parse(slackExpiredTpl)
	if err != nil {
		return fmt.Errorf("parse slack expired template: %w", err)
	}

	// Email templates
	n.emailRequestTemplate, err = template.New("email_request").Parse(emailRequestTpl)
	if err != nil {
		return fmt.Errorf("parse email request template: %w", err)
	}

	n.emailDecisionTemplate, err = template.New("email_decision").Parse(emailDecisionTpl)
	if err != nil {
		return fmt.Errorf("parse email decision template: %w", err)
	}

	n.emailReminderTemplate, err = template.New("email_reminder").Parse(emailReminderTpl)
	if err != nil {
		return fmt.Errorf("parse email reminder template: %w", err)
	}

	n.emailExpiredTemplate, err = template.New("email_expired").Parse(emailExpiredTpl)
	if err != nil {
		return fmt.Errorf("parse email expired template: %w", err)
	}

	return nil
}

// NotifyApprovalRequest sends notifications about a new approval request.
func (n *DefaultNotifier) NotifyApprovalRequest(ctx context.Context, req *Request, channels []string) error {
	data := n.buildTemplateData(req, nil)
	var errs []string

	for _, channel := range channels {
		var err error
		switch {
		case strings.HasPrefix(channel, "slack:"):
			err = n.sendSlackNotification(ctx, channel[6:], n.slackRequestTemplate, data)
		case strings.HasPrefix(channel, "email:"):
			err = n.sendEmailNotification(ctx, channel[6:], "Approval Required: "+req.Title, n.emailRequestTemplate, data)
		case channel == "slack":
			err = n.sendSlackNotification(ctx, n.config.SlackChannel, n.slackRequestTemplate, data)
		default:
			// Try to determine channel type from format
			if strings.Contains(channel, "@") {
				err = n.sendEmailNotification(ctx, channel, "Approval Required: "+req.Title, n.emailRequestTemplate, data)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", channel, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// NotifyApprovalDecision sends notifications about an approval decision.
func (n *DefaultNotifier) NotifyApprovalDecision(ctx context.Context, req *Request, resp Response, channels []string) error {
	data := n.buildTemplateData(req, &resp)
	var errs []string

	subject := fmt.Sprintf("Approval %s: %s", resp.Decision, req.Title)

	for _, channel := range channels {
		var err error
		switch {
		case strings.HasPrefix(channel, "slack:"):
			err = n.sendSlackNotification(ctx, channel[6:], n.slackDecisionTemplate, data)
		case strings.HasPrefix(channel, "email:"):
			err = n.sendEmailNotification(ctx, channel[6:], subject, n.emailDecisionTemplate, data)
		case channel == "slack":
			err = n.sendSlackNotification(ctx, n.config.SlackChannel, n.slackDecisionTemplate, data)
		default:
			if strings.Contains(channel, "@") {
				err = n.sendEmailNotification(ctx, channel, subject, n.emailDecisionTemplate, data)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", channel, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// NotifyApprovalReminder sends reminder notifications for pending approvals.
func (n *DefaultNotifier) NotifyApprovalReminder(ctx context.Context, req *Request, channels []string) error {
	data := n.buildTemplateData(req, nil)
	var errs []string

	for _, channel := range channels {
		var err error
		switch {
		case strings.HasPrefix(channel, "slack:"):
			err = n.sendSlackNotification(ctx, channel[6:], n.slackReminderTemplate, data)
		case strings.HasPrefix(channel, "email:"):
			err = n.sendEmailNotification(ctx, channel[6:], "Reminder: Approval Pending - "+req.Title, n.emailReminderTemplate, data)
		case channel == "slack":
			err = n.sendSlackNotification(ctx, n.config.SlackChannel, n.slackReminderTemplate, data)
		default:
			if strings.Contains(channel, "@") {
				err = n.sendEmailNotification(ctx, channel, "Reminder: Approval Pending - "+req.Title, n.emailReminderTemplate, data)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", channel, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// NotifyApprovalExpired sends notifications that an approval has expired.
func (n *DefaultNotifier) NotifyApprovalExpired(ctx context.Context, req *Request, channels []string) error {
	data := n.buildTemplateData(req, nil)
	var errs []string

	for _, channel := range channels {
		var err error
		switch {
		case strings.HasPrefix(channel, "slack:"):
			err = n.sendSlackNotification(ctx, channel[6:], n.slackExpiredTemplate, data)
		case strings.HasPrefix(channel, "email:"):
			err = n.sendEmailNotification(ctx, channel[6:], "Approval Expired: "+req.Title, n.emailExpiredTemplate, data)
		case channel == "slack":
			err = n.sendSlackNotification(ctx, n.config.SlackChannel, n.slackExpiredTemplate, data)
		default:
			if strings.Contains(channel, "@") {
				err = n.sendEmailNotification(ctx, channel, "Approval Expired: "+req.Title, n.emailExpiredTemplate, data)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", channel, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// templateData contains data for notification templates.
type templateData struct {
	Request       *Request
	Response      *Response
	ApproveURL    string
	RejectURL     string
	ViewURL       string
	TimeRemaining string
}

// buildTemplateData builds template data from a request.
func (n *DefaultNotifier) buildTemplateData(req *Request, resp *Response) templateData {
	data := templateData{
		Request:  req,
		Response: resp,
	}

	if n.config.ApprovalURLBase != "" {
		base := strings.TrimSuffix(n.config.ApprovalURLBase, "/")
		data.ApproveURL = fmt.Sprintf("%s/approve/%s", base, req.ID)
		data.RejectURL = fmt.Sprintf("%s/reject/%s", base, req.ID)
		data.ViewURL = fmt.Sprintf("%s/view/%s", base, req.ID)
	}

	if req.ExpiresAt != nil && req.State == RequestStatePending {
		remaining := time.Until(*req.ExpiresAt)
		if remaining > 0 {
			data.TimeRemaining = formatDuration(remaining)
		} else {
			data.TimeRemaining = "expired"
		}
	}

	return data
}

// sendSlackNotification sends a notification to Slack.
func (n *DefaultNotifier) sendSlackNotification(ctx context.Context, channel string, tpl *template.Template, data templateData) error {
	if n.config.SlackWebhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	// Parse the rendered JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}

	// Set channel if specified
	if channel != "" {
		payload["channel"] = channel
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.config.SlackWebhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

// sendEmailNotification sends an email notification.
func (n *DefaultNotifier) sendEmailNotification(ctx context.Context, to, subject string, tpl *template.Template, data templateData) error {
	if n.config.SMTPHost == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	var body bytes.Buffer
	if err := tpl.Execute(&body, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		n.config.EmailFrom, to, subject, body.String())

	addr := fmt.Sprintf("%s:%d", n.config.SMTPHost, n.config.SMTPPort)

	var auth smtp.Auth
	if n.config.SMTPUser != "" {
		auth = smtp.PlainAuth("", n.config.SMTPUser, n.config.SMTPPassword, n.config.SMTPHost)
	}

	// Use a channel to handle context cancellation
	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, n.config.EmailFrom, []string{to}, []byte(msg))
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// Slack message templates
const slackRequestTpl = `{
	"text": "Approval Required: {{.Request.Title}}",
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "🔔 Approval Required"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*{{.Request.Title}}*\n{{.Request.Description}}"
			}
		},
		{
			"type": "section",
			"fields": [
				{
					"type": "mrkdwn",
					"text": "*Runbook:*\n{{if .Request.Metadata.runbook_name}}{{.Request.Metadata.runbook_name}}{{else}}N/A{{end}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Step:*\n{{.Request.StepName}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Mode:*\n{{.Request.Mode}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Approvers:*\n{{range .Request.Approvers}}• {{.}}\n{{end}}"
				}
			]
		}{{if .TimeRemaining}},
		{
			"type": "context",
			"elements": [
				{
					"type": "mrkdwn",
					"text": "⏱️ Time remaining: {{.TimeRemaining}}"
				}
			]
		}{{end}}{{if .ApproveURL}},
		{
			"type": "actions",
			"elements": [
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "✅ Approve"
					},
					"style": "primary",
					"url": "{{.ApproveURL}}"
				},
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "❌ Reject"
					},
					"style": "danger",
					"url": "{{.RejectURL}}"
				},
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "View Details"
					},
					"url": "{{.ViewURL}}"
				}
			]
		}{{end}}
	]
}`

const slackDecisionTpl = `{
	"text": "Approval {{.Response.Decision}}: {{.Request.Title}}",
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "{{if eq .Response.Decision "approved"}}✅ Approved{{else}}❌ Rejected{{end}}"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*{{.Request.Title}}*"
			}
		},
		{
			"type": "section",
			"fields": [
				{
					"type": "mrkdwn",
					"text": "*Decision:*\n{{.Response.Decision}}"
				},
				{
					"type": "mrkdwn",
					"text": "*By:*\n{{.Response.Approver}}"
				}
			]
		}{{if .Response.Comment}},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*Comment:*\n{{.Response.Comment}}"
			}
		}{{end}}
	]
}`

const slackReminderTpl = `{
	"text": "Reminder: Approval pending for {{.Request.Title}}",
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "⏰ Approval Reminder"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*{{.Request.Title}}*\nThis approval request is still pending your review."
			}
		}{{if .TimeRemaining}},
		{
			"type": "context",
			"elements": [
				{
					"type": "mrkdwn",
					"text": "⏱️ Time remaining: {{.TimeRemaining}}"
				}
			]
		}{{end}}{{if .ApproveURL}},
		{
			"type": "actions",
			"elements": [
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "✅ Approve"
					},
					"style": "primary",
					"url": "{{.ApproveURL}}"
				},
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "❌ Reject"
					},
					"style": "danger",
					"url": "{{.RejectURL}}"
				}
			]
		}{{end}}
	]
}`

const slackExpiredTpl = `{
	"text": "Approval expired: {{.Request.Title}}",
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "⌛ Approval Expired"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*{{.Request.Title}}*\nThis approval request has expired without receiving the required approvals."
			}
		},
		{
			"type": "section",
			"fields": [
				{
					"type": "mrkdwn",
					"text": "*Approvals received:*\n{{.Request.ApprovalCount}}/{{.Request.RequiredCount}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Step:*\n{{.Request.StepName}}"
				}
			]
		}
	]
}`

// Email templates
const emailRequestTpl = `<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #2563eb; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8fafc; padding: 20px; border: 1px solid #e2e8f0; }
        .footer { padding: 20px; border: 1px solid #e2e8f0; border-top: none; border-radius: 0 0 8px 8px; }
        .btn { display: inline-block; padding: 12px 24px; margin: 5px; text-decoration: none; border-radius: 6px; font-weight: 600; }
        .btn-approve { background: #16a34a; color: white; }
        .btn-reject { background: #dc2626; color: white; }
        .btn-view { background: #6b7280; color: white; }
        .info { margin: 10px 0; }
        .label { font-weight: 600; color: #374151; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 style="margin: 0;">🔔 Approval Required</h1>
        </div>
        <div class="content">
            <h2>{{.Request.Title}}</h2>
            {{if .Request.Description}}<p>{{.Request.Description}}</p>{{end}}

            <div class="info">
                <span class="label">Step:</span> {{.Request.StepName}}
            </div>
            <div class="info">
                <span class="label">Mode:</span> {{.Request.Mode}}
            </div>
            <div class="info">
                <span class="label">Approvers:</span>
                <ul>
                {{range .Request.Approvers}}<li>{{.}}</li>{{end}}
                </ul>
            </div>
            {{if .TimeRemaining}}
            <div class="info">
                <span class="label">Time remaining:</span> {{.TimeRemaining}}
            </div>
            {{end}}
        </div>
        <div class="footer">
            {{if .ApproveURL}}
            <a href="{{.ApproveURL}}" class="btn btn-approve">✅ Approve</a>
            <a href="{{.RejectURL}}" class="btn btn-reject">❌ Reject</a>
            <a href="{{.ViewURL}}" class="btn btn-view">View Details</a>
            {{else}}
            <p>Please use the CLI or API to respond to this approval request.</p>
            {{end}}
        </div>
    </div>
</body>
</html>`

const emailDecisionTpl = `<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header-approved { background: #16a34a; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .header-rejected { background: #dc2626; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8fafc; padding: 20px; border: 1px solid #e2e8f0; border-radius: 0 0 8px 8px; }
        .info { margin: 10px 0; }
        .label { font-weight: 600; color: #374151; }
    </style>
</head>
<body>
    <div class="container">
        <div class="{{if eq .Response.Decision "approved"}}header-approved{{else}}header-rejected{{end}}">
            <h1 style="margin: 0;">{{if eq .Response.Decision "approved"}}✅ Approved{{else}}❌ Rejected{{end}}</h1>
        </div>
        <div class="content">
            <h2>{{.Request.Title}}</h2>

            <div class="info">
                <span class="label">Decision:</span> {{.Response.Decision}}
            </div>
            <div class="info">
                <span class="label">By:</span> {{.Response.Approver}}
            </div>
            {{if .Response.Comment}}
            <div class="info">
                <span class="label">Comment:</span> {{.Response.Comment}}
            </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

const emailReminderTpl = `<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #f59e0b; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8fafc; padding: 20px; border: 1px solid #e2e8f0; }
        .footer { padding: 20px; border: 1px solid #e2e8f0; border-top: none; border-radius: 0 0 8px 8px; }
        .btn { display: inline-block; padding: 12px 24px; margin: 5px; text-decoration: none; border-radius: 6px; font-weight: 600; }
        .btn-approve { background: #16a34a; color: white; }
        .btn-reject { background: #dc2626; color: white; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 style="margin: 0;">⏰ Approval Reminder</h1>
        </div>
        <div class="content">
            <h2>{{.Request.Title}}</h2>
            <p>This approval request is still pending your review.</p>
            {{if .TimeRemaining}}
            <p><strong>Time remaining:</strong> {{.TimeRemaining}}</p>
            {{end}}
        </div>
        <div class="footer">
            {{if .ApproveURL}}
            <a href="{{.ApproveURL}}" class="btn btn-approve">✅ Approve</a>
            <a href="{{.RejectURL}}" class="btn btn-reject">❌ Reject</a>
            {{end}}
        </div>
    </div>
</body>
</html>`

const emailExpiredTpl = `<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #6b7280; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8fafc; padding: 20px; border: 1px solid #e2e8f0; border-radius: 0 0 8px 8px; }
        .info { margin: 10px 0; }
        .label { font-weight: 600; color: #374151; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 style="margin: 0;">⌛ Approval Expired</h1>
        </div>
        <div class="content">
            <h2>{{.Request.Title}}</h2>
            <p>This approval request has expired without receiving the required approvals.</p>

            <div class="info">
                <span class="label">Approvals received:</span> {{.Request.ApprovalCount}}/{{.Request.RequiredCount}}
            </div>
            <div class="info">
                <span class="label">Step:</span> {{.Request.StepName}}
            </div>
        </div>
    </div>
</body>
</html>`
