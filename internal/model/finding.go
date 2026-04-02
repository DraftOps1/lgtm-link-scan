package model

type Finding struct {
	Severity       string `json:"severity"`
	Title          string `json:"title"`
	Fact           string `json:"fact"`
	Impact         string `json:"impact"`
	Recommendation string `json:"recommendation"`
}
