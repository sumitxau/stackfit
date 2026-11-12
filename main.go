package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"
)

const (
	seenFilePath    = "seen_jobs.json"
	appliedFilePath = "applied_jobs.json"
	pitchTmplPath   = "pitch.tmpl"
	maxJobsPerRun   = 9
	retentionDays   = 60
)

type AppliedTracker struct {
	AppliedCompanies []string `json:"applied_companies"`
	AppliedURLs      []string `json:"applied_urls"`
}

func loadSeenState() map[string]string {
	seen := make(map[string]string)
	data, err := os.ReadFile(seenFilePath)
	if err != nil {
		return seen
	}
	_ = json.Unmarshal(data, &seen)

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	for id, ts := range seen {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			if parsed.Before(cutoff) {
				delete(seen, id)
			}
		}
	}
	return seen
}

func saveSeenState(seen map[string]string) error {
	data, err := json.MarshalIndent(seen, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(seenFilePath, data, 0644)
}

func loadAppliedTracker() AppliedTracker {
	var tracker AppliedTracker
	data, err := os.ReadFile(appliedFilePath)
	if err != nil {
		return tracker
	}
	_ = json.Unmarshal(data, &tracker)
	return tracker
}

func isAlreadyApplied(job JobListing, tracker AppliedTracker) bool {
	compLower := strings.ToLower(strings.TrimSpace(job.Company))
	for _, c := range tracker.AppliedCompanies {
		if strings.ToLower(strings.TrimSpace(c)) == compLower {
			return true
		}
	}
	for _, u := range tracker.AppliedURLs {
		if strings.TrimSpace(u) == job.URL {
			return true
		}
	}
	return false
}

func main() {
	if os.Getenv("GITHUB_ACTIONS") == "true" && strings.TrimSpace(os.Getenv("DISCORD_WEBHOOK")) == "" {
		fmt.Fprintln(os.Stderr, "[ERROR] DISCORD_WEBHOOK secret is empty in GitHub Actions.")
		os.Exit(1)
	}

	tmpl, err := template.ParseFiles(pitchTmplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] failed to parse %s: %v\n", pitchTmplPath, err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	seen := loadSeenState()
	applied := loadAppliedTracker()

	jobs, err := CollectJobsConcurrently(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[INFO] Fetched %d eligible entry/mid-level jobs.\n", len(jobs))

	webhookURL := os.Getenv("DISCORD_WEBHOOK")
	sentCount := 0

	for _, job := range jobs {
		if _, exists := seen[job.ID]; exists {
			continue
		}
		if isAlreadyApplied(job, applied) {
			continue
		}

		delivered, err := DispatchToDiscord(client, webhookURL, tmpl, job)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] failed to dispatch %s: %v\n", job.ID, err)
			continue
		}

		if delivered {
			seen[job.ID] = time.Now().UTC().Format(time.RFC3339)
			if err := saveSeenState(seen); err != nil {
				fmt.Fprintf(os.Stderr, "[WARN] failed to save state: %v\n", err)
			}
			sentCount++
			fmt.Printf("[SENT %d/%d] %s - %s\n", sentCount, maxJobsPerRun, job.Company, job.Title)
			time.Sleep(1500 * time.Millisecond)
		} else {
			sentCount++
		}

		if sentCount >= maxJobsPerRun {
			break
		}
	}

	fmt.Printf("[INFO] Run finished. Dispatched %d jobs.\n", sentCount)
}
