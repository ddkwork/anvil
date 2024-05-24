package regex

import (
	"testing"

	"github.com/ddkwork/golibrary/mylog"
	"github.com/stretchr/testify/assert"
)

func TestParseRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *astnode
		ok       bool
		errors   []error
	}{
		{
			name:  "abc",
			input: "abc",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{typ: literalNode, tok: token{typ: literalTok, pos: 0, value: []rune{'a'}}},
							{typ: literalNode, tok: token{typ: literalTok, pos: 1, value: []rune{'b'}}},
							{typ: literalNode, tok: token{typ: literalTok, pos: 2, value: []rune{'c'}}},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "^abc",
			input: "^abc",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{typ: basicAnchorNode, tok: token{typ: basicAnchorTok, pos: 0, value: []rune{'^'}}},
							{typ: literalNode, tok: token{typ: literalTok, pos: 1, value: []rune{'a'}}},
							{typ: literalNode, tok: token{typ: literalTok, pos: 2, value: []rune{'b'}}},
							{typ: literalNode, tok: token{typ: literalTok, pos: 3, value: []rune{'c'}}},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "a*c",
			input: "a*c",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{
								typ: repetitionNode,
								tok: token{typ: repetitionTok, pos: 1, value: []rune{'*'}},
								children: []*astnode{
									{typ: literalNode, tok: token{typ: literalTok, pos: 0, value: []rune{'a'}}},
								},
							},
							{typ: literalNode, tok: token{typ: literalTok, pos: 2, value: []rune{'c'}}},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "ab|cd",
			input: "ab|cd",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: alternativesNode,
						children: []*astnode{
							{
								typ: termsNode,
								children: []*astnode{
									{typ: literalNode, tok: token{typ: literalTok, pos: 0, value: []rune{'a'}}},
									{typ: literalNode, tok: token{typ: literalTok, pos: 1, value: []rune{'b'}}},
								},
							},

							{
								typ: termsNode,
								children: []*astnode{
									{typ: literalNode, tok: token{typ: literalTok, pos: 3, value: []rune{'c'}}},
									{typ: literalNode, tok: token{typ: literalTok, pos: 4, value: []rune{'d'}}},
								},
							},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "ab|cd|.",
			input: "ab|cd|.",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: alternativesNode,
						children: []*astnode{
							{
								typ: termsNode,
								children: []*astnode{
									{typ: literalNode, tok: token{typ: literalTok, pos: 0, value: []rune{'a'}}},
									{typ: literalNode, tok: token{typ: literalTok, pos: 1, value: []rune{'b'}}},
								},
							},

							{
								typ: termsNode,
								children: []*astnode{
									{typ: literalNode, tok: token{typ: literalTok, pos: 3, value: []rune{'c'}}},
									{typ: literalNode, tok: token{typ: literalTok, pos: 4, value: []rune{'d'}}},
								},
							},

							{
								typ: termsNode,
								children: []*astnode{
									{typ: literalNode, tok: token{typ: literalTok, pos: 6, value: []rune{'.'}}},
								},
							},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "(ab)*",
			input: "(ab)*",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{
								typ: repetitionNode,
								tok: token{typ: repetitionTok, pos: 4, value: []rune{'*'}},
								children: []*astnode{
									{
										typ: numberedGroupNode,
										tok: token{typ: openNumberedGroupTok, pos: 0, value: []rune{'('}},
										children: []*astnode{
											{
												typ: termsNode,
												children: []*astnode{
													{typ: literalNode, tok: token{typ: literalTok, pos: 1, value: []rune{'a'}}},
													{typ: literalNode, tok: token{typ: literalTok, pos: 2, value: []rune{'b'}}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "(?:ab)*",
			input: "(?:ab)*",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{
								typ: repetitionNode,
								tok: token{typ: repetitionTok, pos: 6, value: []rune{'*'}},
								children: []*astnode{
									{
										typ: numberedGroupNode,
										tok: token{typ: openNumberedGroupTok, pos: 0, value: []rune("(?:")},
										children: []*astnode{
											{
												typ: termsNode,
												children: []*astnode{
													{typ: literalNode, tok: token{typ: literalTok, pos: 3, value: []rune{'a'}}},
													{typ: literalNode, tok: token{typ: literalTok, pos: 4, value: []rune{'b'}}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "(?i)a",
			input: "(?i)a",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{
								typ: flagsNode,
								tok: token{typ: flagsTok, pos: 0, value: []rune("(?i)")},
							},

							{typ: literalNode, tok: token{typ: literalTok, pos: 4, value: []rune{'a'}}},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "(?P<splort>(?m:(ab)))",
			input: "(?P<splort>(?m:(ab)))",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{
								typ: namedGroupNode,
								tok: token{typ: openNamedGroupTok, pos: 0, value: []rune("(?P<splort>")},
								children: []*astnode{
									{
										typ: termsNode,
										children: []*astnode{
											{
												typ: flagsGroupNode,
												tok: token{typ: openFlagsGroupTok, pos: 11, value: []rune("(?m:")},
												children: []*astnode{
													{
														typ: termsNode,
														children: []*astnode{
															{
																typ: numberedGroupNode,
																tok: token{typ: openNumberedGroupTok, pos: 15, value: []rune("(")},
																children: []*astnode{
																	{
																		typ: termsNode,
																		children: []*astnode{
																			{typ: literalNode, tok: token{typ: literalTok, pos: 16, value: []rune{'a'}}},
																			{typ: literalNode, tok: token{typ: literalTok, pos: 17, value: []rune{'b'}}},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "[[:digit:]][a-z]",
			input: "[[:digit:]][a-z]",
			expected: &astnode{
				typ: rootNode,
				children: []*astnode{
					{
						typ: termsNode,
						children: []*astnode{
							{
								typ: classOrEscapeNode,
								tok: token{typ: classOrEscapeTok, pos: 0, value: []rune("[[:digit:]]")},
							},
							{
								typ: classOrEscapeNode,
								tok: token{typ: classOrEscapeTok, pos: 11, value: []rune("[a-z]")},
							},
						},
					},
				},
			},
			ok:     true,
			errors: []error{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var s scanner
			toks, ok := s.Scan(tc.input)

			if ok != tc.ok {
				t.Fatalf("Scan returned ok=%v but expected %v. errors: %v", ok, tc.ok, s.errs)
			}

			var p parser
			tree := mylog.Check2(p.Parse(toks))
			if err != nil && !tc.ok {
				t.Fatalf("Parse returned an error: %v", err)
			}

			if ok {
				b := assert.Equal(t, tc.expected, tree)
				if !b {
					t.Logf("parsed nodes are: \n%s\n", tree)
					t.Logf("expected nodes are: \n%s\n", tc.expected)
				}
			}

			assert.Equal(t, tc.errors, s.errs)
		})
	}
}
