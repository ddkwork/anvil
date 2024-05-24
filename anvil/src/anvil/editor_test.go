package main

import "testing"

func TestRemoveTagFromString(t *testing.T) {
	tests := []struct {
		name   string
		tag    string
		job    string
		output string
	}{
		{
			name:   "not in tag",
			tag:    "Newcol",
			job:    "ls",
			output: "Newcol",
		},
		{
			name:   "ls first",
			tag:    "ls Newcol",
			job:    "ls",
			output: "Newcol",
		},
		{
			name:   "ls first part 2",
			tag:    "ls sleep Newcol",
			job:    "ls",
			output: "sleep Newcol",
		},
		{
			name:   "ls middle",
			tag:    "sleep ls Newcol",
			job:    "ls",
			output: "sleep Newcol",
		},
		{
			name:   "ls last",
			tag:    "sleep ls",
			job:    "ls",
			output: "sleep",
		},
		{
			name:   "ls last part 2",
			tag:    "sleep ls ",
			job:    "ls",
			output: "sleep",
		},
		{
			name:   "ls only",
			tag:    "ls",
			job:    "ls",
			output: "",
		},
		{
			name:   "ls only part 2",
			tag:    "ls ",
			job:    "ls",
			output: "",
		},
		{
			name:   "job is substring 1",
			tag:    "tmp+Errors tmp Newcol",
			job:    "tmp",
			output: "tmp+Errors Newcol",
		},
		{
			name:   "job is substring 2",
			tag:    "tmp tmp+Errors Newcol",
			job:    "tmp",
			output: "tmp+Errors Newcol",
		},
		{
			name:   "job is substring 3",
			tag:    "tmp+Errors tmp Newcol",
			job:    "tmp+Errors",
			output: "tmp Newcol",
		},
		{
			name:   "job is substring 4",
			tag:    "tmp+Errors tmp",
			job:    "tmp",
			output: "tmp+Errors",
		},
		{
			name:   "job is substring 5",
			tag:    "boot oo Newcol",
			job:    "oo",
			output: "boot Newcol",
		},
		{
			name:   "job is substring 6",
			tag:    "tmp tmp+Errors tmp",
			job:    "tmp",
			output: "tmp+Errors tmp",
		},
		{
			name:   "job is substring 6",
			tag:    "boo oo Newcol",
			job:    "oo",
			output: "boo Newcol",
		},
		{
			name:   "job is substring 7",
			tag:    "oo boo Newcol",
			job:    "oo",
			output: "boo Newcol",
		},
		{
			name:   "job is substring 8",
			tag:    "a oo boo Newcol",
			job:    "oo",
			output: "a boo Newcol",
		},
		{
			name:   "job is substring 9",
			tag:    "a oo boo",
			job:    "oo",
			output: "a boo",
		},
		{
			name:   "job is substring 9",
			tag:    "a boo oo",
			job:    "oo",
			output: "a boo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _, _ := removeJobFromTagString(tc.job, tc.tag)

			if result != tc.output {
				t.Fatalf("Expected '%s' but got '%s'", tc.output, result)
			}
		})
	}
}
