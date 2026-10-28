package core

import "github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"

func leanReport() lean.Report {
	return lean.Report{
		Issues: []lean.Issue{
			{File: "f.json", Path: "a", Message: "m1", Hint: "h1", Closest: "Spear"},
			{File: "f.json", Path: "b", Message: "m2"},
			{File: "f.json", Path: "c", Message: "m3"},
			{File: "f.json", Path: "d", Message: "m4"},
			{File: "f.json", Path: "e", Message: "w", Severity: lean.Warn},
		},
		Truncated: 7,
	}
}

func leanReportN(n int) lean.Report {
	var r lean.Report
	for i := 0; i < n; i++ {
		r.Errorf("f.json", "p", "m")
	}
	return r
}
