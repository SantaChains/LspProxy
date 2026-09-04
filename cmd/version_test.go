package cmd

import "testing"

func TestParsePseudoVersion(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		commit string
		date   string
	}{
		{
			name:   "标准伪版本",
			input:  "v0.0.0-20260904150232-44a41b010415",
			commit: "44a41b010415",
			date:   "2026-09-04",
		},
		{
			name:   "带 pre 段的伪版本",
			input:  "v1.2.3-pre.0.20260101000000-abcdef123456",
			commit: "abcdef123456",
			date:   "2026-01-01",
		},
		{
			name:   "语义化版本（非伪版本）",
			input:  "v1.2.3",
			commit: "",
			date:   "",
		},
		{
			name:   "预发布 tag",
			input:  "v1.2.3-rc1",
			commit: "",
			date:   "",
		},
		{
			name:   "时间戳位数不足",
			input:  "v0.0.0-20260904-44a41b010415",
			commit: "",
			date:   "",
		},
		{
			name:   "commit 非十六进制",
			input:  "v0.0.0-20260904150232-44a41b01041g",
			commit: "",
			date:   "",
		},
		{
			name:   "devel",
			input:  "(devel)",
			commit: "",
			date:   "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			commit, date := parsePseudoVersion(c.input)
			if commit != c.commit {
				t.Errorf("commit = %q, want %q", commit, c.commit)
			}
			if date != c.date {
				t.Errorf("date = %q, want %q", date, c.date)
			}
		})
	}
}
