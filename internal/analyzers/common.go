package analyzers

import (
	"math"
	"sort"
)

func buildCategoryScore(id, title string, earned, available int) CategoryScoreValues {
	percent := 0
	status := "fail"
	if available > 0 {
		percent = int(math.Round(float64(earned) * 100 / float64(available)))
	}
	if percent >= 80 {
		status = "pass"
	} else if percent >= 50 {
		status = "warn"
	}

	return CategoryScoreValues{
		ID:        id,
		Title:     title,
		Earned:    earned,
		Available: available,
		Percent:   percent,
		Status:    status,
	}
}

type CategoryScoreValues struct {
	ID        string
	Title     string
	Earned    int
	Available int
	Percent   int
	Status    string
}

func severityForCoverage(value float64) string {
	switch {
	case value < 50:
		return "HIGH"
	case value < 80:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func percentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*1000) / 10
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
