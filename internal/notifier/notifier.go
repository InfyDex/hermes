package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"time"

	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/models"
)

type EventType string

const (
	EventStart   EventType = "start"
	EventSuccess EventType = "success"
	EventFailure EventType = "failure"
	EventCancel  EventType = "cancel"
)

type Notifier struct {
	db         *database.DB
	cfg        *config.NotifyConfig
	url        string
	serverName string
}

func New(db *database.DB, cfg *config.NotifyConfig, domainURL, serverName string) *Notifier {
	return &Notifier{
		db:         db,
		cfg:        cfg,
		url:        domainURL,
		serverName: serverName,
	}
}

func (n *Notifier) Notify(job *models.Job, exec *models.Execution, event EventType) {
	if !n.isEventEnabled(job, event) {
		return
	}

	title, color, level := n.getEventDetails(job, exec, event)

	var actionLink string
	if n.url != "" {
		if exec != nil && exec.ID > 0 {
			actionLink = fmt.Sprintf("%s/executions/%d/logs", n.url, exec.ID)
		} else {
			actionLink = fmt.Sprintf("%s/jobs/%d", n.url, job.ID)
		}
	}

	if job.NotifyWeb {
		dbMessage := title
		if actionLink != "" {
			dbMessage = fmt.Sprintf("%s - %s", title, actionLink)
		}
		if err := n.db.InsertNotification(job.ID, level, dbMessage); err != nil {
			log.Printf("Failed to insert web notification: %v", err)
		}
	}

	if job.NotifyDiscord && n.cfg.DiscordWebhookURL != "" {
		go n.sendDiscord(n.cfg.DiscordWebhookURL, title, actionLink, color, job.Name)
	}

	if job.NotifyEmail && n.cfg.SMTPHost != "" && n.cfg.SMTPUser != "" {
		go n.sendEmail(title, actionLink, job.Name, event)
	}
}

func (n *Notifier) isEventEnabled(job *models.Job, event EventType) bool {
	switch event {
	case EventStart:
		return job.NotifyOnStart
	case EventSuccess:
		return job.NotifyOnSuccess
	case EventFailure:
		return job.NotifyOnFailure
	case EventCancel:
		return job.NotifyOnCancel
	default:
		return false
	}
}

func (n *Notifier) getEventDetails(job *models.Job, exec *models.Execution, event EventType) (string, int, string) {
	switch event {
	case EventStart:
		return fmt.Sprintf("Job Started: %s", job.Name), 3447003, "info"
	case EventSuccess:
		dur := ""
		if exec != nil && exec.EndTime.Valid {
			dur = fmt.Sprintf(" in %v", exec.EndTime.Time.Sub(exec.StartTime).Round(time.Second))
		}
		return fmt.Sprintf("Job Succeeded: %s%s", job.Name, dur), 3066993, "success"
	case EventFailure:
		return fmt.Sprintf("Job Failed: %s", job.Name), 15158332, "error"
	case EventCancel:
		return fmt.Sprintf("Job Canceled: %s", job.Name), 15105570, "warning"
	default:
		return fmt.Sprintf("Job Update: %s", job.Name), 9807270, "info"
	}
}

type Embed struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Color       int    `json:"color"`
	URL         string `json:"url,omitempty"`
}

type Payload struct {
	Username string  `json:"username,omitempty"`
	Embeds   []Embed `json:"embeds"`
}

func (n *Notifier) sendDiscord(webhookURL, title, link string, color int, jobName string) {
	desc := ""
	if link != "" {
		desc = fmt.Sprintf("[View Execution Logs here](%s)", link)
	}

	payload := Payload{
		Embeds: []Embed{
			{
				Title:       title,
				Description: desc,
				Color:       color,
				URL:         link,
			},
		},
	}

	if n.serverName != "" {
		payload.Username = fmt.Sprintf("Hermes - %s", n.serverName)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal discord payload: %v", err)
		return
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to send discord webhook: %v", err)
		return
	}
	defer resp.Body.Close()
}

func (n *Notifier) sendEmail(title, link, jobName string, event EventType) {
	headers := make(map[string]string)
	from := n.cfg.SMTPFrom
	if from == "" {
		from = n.cfg.SMTPUser
	}
	headers["From"] = from
	headers["To"] = n.cfg.SMTPUser

	subjectPrefix := "Hermes"
	if n.serverName != "" {
		subjectPrefix = fmt.Sprintf("Hermes - %s", n.serverName)
	}
	headers["Subject"] = subjectPrefix

	headers["MIME-version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"UTF-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n"
	body := fmt.Sprintf("<h2>%s</h2>", title)
	if link != "" {
		body += fmt.Sprintf("<p><a href='%s'>View details in Hermes</a></p>", link)
	}
	message += body

	auth := smtp.PlainAuth("", n.cfg.SMTPUser, n.cfg.SMTPPass, n.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", n.cfg.SMTPHost, n.cfg.SMTPPort)
	if n.cfg.SMTPPort == 0 {
		addr = fmt.Sprintf("%s:587", n.cfg.SMTPHost)
	}

	err := smtp.SendMail(addr, auth, from, []string{n.cfg.SMTPUser}, []byte(message))
	if err != nil {
		log.Printf("Failed to send email: %v", err)
	}
}

func (n *Notifier) SystemNotify(title, message string) {
	level := "info"
	color := 3447003 // Blue

	// Web UI
	if err := n.db.InsertNotification(0, level, title+" - "+message); err != nil {
		log.Printf("Failed to insert system web notification: %v", err)
	}

	// Discord
	if n.cfg.DiscordWebhookURL != "" {
		go n.sendDiscord(n.cfg.DiscordWebhookURL, title, "", color, "System")
	}

	// Email
	if n.cfg.SMTPHost != "" && n.cfg.SMTPUser != "" {
		go n.sendSystemEmail(title, message)
	}
}

func (n *Notifier) sendSystemEmail(title, bodyText string) {
	headers := make(map[string]string)
	from := n.cfg.SMTPFrom
	if from == "" {
		from = n.cfg.SMTPUser
	}
	headers["From"] = from
	headers["To"] = n.cfg.SMTPUser

	subjectPrefix := "Hermes"
	if n.serverName != "" {
		subjectPrefix = fmt.Sprintf("Hermes - %s", n.serverName)
	}
	headers["Subject"] = subjectPrefix

	headers["MIME-version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"UTF-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n"
	body := fmt.Sprintf("<h2>%s</h2><p>%s</p>", title, bodyText)
	message += body

	auth := smtp.PlainAuth("", n.cfg.SMTPUser, n.cfg.SMTPPass, n.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", n.cfg.SMTPHost, n.cfg.SMTPPort)
	if n.cfg.SMTPPort == 0 {
		addr = fmt.Sprintf("%s:587", n.cfg.SMTPHost)
	}

	err := smtp.SendMail(addr, auth, from, []string{n.cfg.SMTPUser}, []byte(message))
	if err != nil {
		log.Printf("Failed to send system email: %v", err)
	}
}
