package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/template"
)

type PitchData struct {
	Company      string
	Title        string
	ResumeFile   string
	ContactPhone string
}

func sanitizeBackticks(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func RenderPitch(tmpl *template.Template, job JobListing) (string, error) {
	data := PitchData{
		Company:      sanitizeBackticks(job.Company),
		Title:        sanitizeBackticks(job.Title),
		ResumeFile:   job.ResumeType,
		ContactPhone: strings.TrimSpace(os.Getenv("CONTACT_PHONE")),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func DispatchToDiscord(client *http.Client, webhookURL string, tmpl *template.Template, job JobListing) (bool, error) {
	pitch, err := RenderPitch(tmpl, job)
	if err != nil {
		return false, err
	}

	trimmedURL := strings.TrimSpace(webhookURL)
	if trimmedURL == "" {
		fmt.Printf("\n[DRY-RUN] %s | %s (%s)\nURL: %s\nAttach: %s\n",
			job.Company, job.Title, job.Source, job.URL, job.ResumeType)
		return false, nil
	}

	content := fmt.Sprintf(
		"**[MATCH] %s - %s**\nSource: %s\nApply URL: <%s>\nAttach Resume: `%s`\n```text\n%s\n```",
		sanitizeBackticks(job.Company),
		sanitizeBackticks(job.Title),
		job.Source,
		job.URL,
		job.ResumeType,
		pitch,
	)

	payload := map[string]interface{}{
		"username": "StackFit Job Pipeline",
		"content":  content,
		"allowed_mentions": map[string]interface{}{
			"parse": []string{},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", trimmedURL, bytes.NewBuffer(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("discord webhook returned HTTP %d", resp.StatusCode)
	}

	return true, nil
}
