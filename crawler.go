package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
	// "time"
)

type JobListing struct {
	ID         string
	Company    string
	Title      string
	URL        string
	Source     string
	ResumeType string
}

var (
	stackRegex    = regexp.MustCompile(`(?i)\b(typescript|next\.?js|react\.?js|react|node\.?js|express\.?js|full[- ]?stack|solana|web3)\b`)
	web3Regex     = regexp.MustCompile(`(?i)\b(solana|web3|defi|smart contract|spl|anchor)\b`)
	seniorExclude = regexp.MustCompile(`(?i)\b(senior|sr\.?|staff|principal|lead|director|head of|vp|manager|architect|chief)\b`)
	htmlTagStrip  = regexp.MustCompile(`<[^>]*>`)
)

func IsEligibleJob(title, tagsAndDesc string) bool {
	if title == "" {
		return false
	}
	if seniorExclude.MatchString(title) {
		return false
	}
	combined := title + " " + tagsAndDesc
	return stackRegex.MatchString(combined)
}

func SelectResume(title, tagsAndDesc string) string {
	if web3Regex.MatchString(title + " " + tagsAndDesc) {
		return "resume.pdf"
	}
	return "resume_web2.pdf"
}

func cleanText(s string) string {
	unescaped := html.UnescapeString(s)
	stripped := htmlTagStrip.ReplaceAllString(unescaped, " ")
	return strings.Join(strings.Fields(stripped), " ")
}

func fetchRemoteOK(client *http.Client) ([]JobListing, error) {
	req, err := http.NewRequest("GET", "https://remoteok.com/api", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "StackFit-Go-Client/1.0 (+https://github.com/sumitxau/stackfit)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remoteok status %d", resp.StatusCode)
	}

	var raw []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	var out []JobListing
	for _, item := range raw {
		company, _ := item["company"].(string)
		position, _ := item["position"].(string)
		url, _ := item["url"].(string)
		desc, _ := item["description"].(string)
		idVal := fmt.Sprintf("rok_%v", item["id"])

		var tagList []string
		if rawTags, ok := item["tags"].([]interface{}); ok {
			for _, t := range rawTags {
				if ts, ok := t.(string); ok {
					tagList = append(tagList, ts)
				}
			}
		}
		metaText := strings.Join(tagList, " ") + " " + cleanText(desc)

		if company == "" || position == "" || url == "" {
			continue
		}
		if !IsEligibleJob(position, metaText) {
			continue
		}

		out = append(out, JobListing{
			ID:         idVal,
			Company:    strings.TrimSpace(company),
			Title:      strings.TrimSpace(position),
			URL:        url,
			Source:     "RemoteOK (remoteok.com)",
			ResumeType: SelectResume(position, metaText),
		})
	}
	return out, nil
}

func fetchArbeitnow(client *http.Client) ([]JobListing, error) {
	req, err := http.NewRequest("GET", "https://www.arbeitnow.com/api/job-board-api", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "StackFit-Go-Client/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("arbeitnow status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			Slug        string   `json:"slug"`
			CompanyName string   `json:"company_name"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Remote      bool     `json:"remote"`
			URL         string   `json:"url"`
			Tags        []string `json:"tags"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var out []JobListing
	for _, j := range payload.Data {
		if !j.Remote || j.CompanyName == "" || j.Title == "" || j.URL == "" {
			continue
		}
		metaText := strings.Join(j.Tags, " ") + " " + cleanText(j.Description)
		if !IsEligibleJob(j.Title, metaText) {
			continue
		}
		out = append(out, JobListing{
			ID:         "arb_" + j.Slug,
			Company:    strings.TrimSpace(j.CompanyName),
			Title:      strings.TrimSpace(j.Title),
			URL:        j.URL,
			Source:     "Arbeitnow Remote",
			ResumeType: SelectResume(j.Title, metaText),
		})
	}
	return out, nil
}

func fetchHNWhoIsHiring(client *http.Client) ([]JobListing, error) {
	threadURL := "https://hn.algolia.com/api/v1/search_by_date?tags=story,author_whoishiring&query=Who%20is%20hiring&hitsPerPage=1"
	resp, err := client.Get(threadURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hn thread lookup status %d", resp.StatusCode)
	}

	var stories struct {
		Hits []struct {
			ObjectID string `json:"objectID"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stories); err != nil || len(stories.Hits) == 0 {
		return nil, fmt.Errorf("no hn hiring thread found")
	}

	storyID := stories.Hits[0].ObjectID
	commentsURL := fmt.Sprintf("https://hn.algolia.com/api/v1/search_by_date?tags=comment,story_%s&hitsPerPage=50", storyID)
	cResp, err := client.Get(commentsURL)
	if err != nil {
		return nil, err
	}
	defer cResp.Body.Close()

	if cResp.StatusCode < 200 || cResp.StatusCode >= 300 {
		return nil, fmt.Errorf("hn comments status %d", cResp.StatusCode)
	}

	var comments struct {
		Hits []struct {
			ObjectID    string `json:"objectID"`
			CommentText string `json:"comment_text"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(cResp.Body).Decode(&comments); err != nil {
		return nil, err
	}

	var out []JobListing
	for _, c := range comments.Hits {
		plain := cleanText(c.CommentText)
		if len(plain) < 40 {
			continue
		}
		header := plain
		if len(header) > 140 {
			header = header[:140]
		}
		parts := strings.Split(header, "|")
		company := strings.TrimSpace(parts[0])
		if len(company) > 45 {
			company = company[:45]
		}
		roleTitle := "Full-Stack / Software Engineer"
		if len(parts) > 1 {
			roleTitle = strings.TrimSpace(parts[1])
			if len(roleTitle) > 60 {
				roleTitle = roleTitle[:60]
			}
		}

		if !IsEligibleJob(roleTitle, plain) {
			continue
		}
		if !strings.Contains(strings.ToLower(plain), "remote") && !strings.Contains(strings.ToLower(plain), "india") && !strings.Contains(strings.ToLower(plain), "bengaluru") && !strings.Contains(strings.ToLower(plain), "bangalore") {
			continue
		}

		out = append(out, JobListing{
			ID:         "hn_" + c.ObjectID,
			Company:    company,
			Title:      roleTitle,
			URL:        fmt.Sprintf("https://news.ycombinator.com/item?id=%s", c.ObjectID),
			Source:     "Hacker News Who Is Hiring",
			ResumeType: SelectResume(roleTitle, plain),
		})
	}
	return out, nil
}

func CollectJobsConcurrently(client *http.Client) ([]JobListing, error) {
	type fetchFn func(*http.Client) ([]JobListing, error)
	sources := []fetchFn{fetchRemoteOK, fetchArbeitnow, fetchHNWhoIsHiring}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		allJobs   []JobListing
		errCount  int
		lastError error
	)

	for _, fn := range sources {
		wg.Add(1)
		go func(f fetchFn) {
			defer wg.Done()
			jobs, err := f(client)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errCount++
				lastError = err
				fmt.Printf("[WARN] source fetch error: %v\n", err)
				return
			}
			allJobs = append(allJobs, jobs...)
		}(fn)
	}

	wg.Wait()
	if errCount == len(sources) {
		return nil, fmt.Errorf("all %d job sources failed (last error: %v)", errCount, lastError)
	}
	return allJobs, nil
}
