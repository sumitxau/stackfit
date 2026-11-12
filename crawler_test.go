package main

import "testing"

func TestIsEligibleJob(t *testing.T) {
	cases := []struct {
		name     string
		title    string
		meta     string
		expected bool
	}{
		{
			name:     "Matches Junior TypeScript Full Stack",
			title:    "Junior Full Stack Developer",
			meta:     "TypeScript Next.js PostgreSQL",
			expected: true,
		},
		{
			name:     "Rejects Senior Title",
			title:    "Senior Full Stack Engineer",
			meta:     "TypeScript React Node.js",
			expected: false,
		},
		{
			name:     "Rejects False Substring Trust and Reactive",
			title:    "Trust and Safety Specialist",
			meta:     "Reactive support workflows and define policies",
			expected: false,
		},
		{
			name:     "Matches Solana Engineer",
			title:    "Software Engineer - Web3",
			meta:     "Solana TypeScript React",
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsEligibleJob(tc.title, tc.meta)
			if got != tc.expected {
				t.Errorf("IsEligibleJob(%q, %q) = %v; want %v", tc.title, tc.meta, got, tc.expected)
			}
		})
	}
}
