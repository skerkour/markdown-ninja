// Copyright (C) 2011 Florian Weimer <fw@deneb.enyo.de>

package pcre

import (
	"testing"
)

func TestCompile(t *testing.T) {
	check := func(p string, groups int) {
		re, err := Compile(p, 0)
		if err != nil {
			t.Error(p, err)
		}
		if g := re.Groups(); g != groups {
			t.Error(p, g)
		}
	}
	check("", 0)
	check("^", 0)
	check("^$", 0)
	check("()", 1)
	check("(())", 2)
	check("((?:))", 1)
}

func TestCompileFail(t *testing.T) {
	check := func(p, msg string) {
		_, err := Compile(p, 0)

		switch {
		case err == nil:
			t.Error(p)
		case err.Error() != msg:
			t.Error(p, "Message:", err.Error())
		}
	}

	check("(", "( (1): missing )")
	check(`\`, `\ (1): \ at end of pattern`)
	check(`abc\`, `abc\ (4): \ at end of pattern`)
	check("abc\000", "abc\000 (3): NUL byte in pattern")
	check("a\000bc", "a\000bc (1): NUL byte in pattern")
}

func checkmatch1(t *testing.T, dostring bool, m *Matcher,
	pattern, subject string, args ...interface{},
) {
	re := MustCompile(pattern, 0)
	var prefix string

	if dostring {
		prefix = "string"

		if m == nil {
			m = re.NewMatcherString(subject, 0)
		} else {
			m.ResetString(re, subject, 0)
		}
	} else {
		prefix = "[]byte"

		if m == nil {
			m = re.NewMatcher([]byte(subject), 0)
		} else {
			m.Reset(re, []byte(subject), 0)
		}
	}

	if len(args) == 0 {
		if m.Matches {
			t.Errorf("prefix='%s' pattern='%s' subject='%s': %s", prefix, pattern, subject, "!Matches")
		}

		return
	}

	if !m.Matches {
		t.Logf("%v", m.Matches)
		t.Errorf("prefix='%s' pattern='%s' subject='%s': %s", prefix, pattern, subject, "Matches")

		return
	}

	if m.Groups != len(args)-1 {
		t.Error(prefix, pattern, subject, "Groups", m.Groups)

		return
	}

	for i, arg := range args {
		if s, ok := arg.(string); ok {
			if !m.Present(i) {
				t.Error(prefix, pattern, subject,
					"Present", i)
			}

			if g := string(m.Group(i)); g != s {
				t.Error(prefix, pattern, subject,
					"Group", i, g, "!=", s)
			}

			if g := m.GroupString(i); g != s {
				t.Error(prefix, pattern, subject,
					"GroupString", i, g, "!=", s)
			}
		} else if m.Present(i) {
			t.Error(prefix, pattern, subject,
				"!Present", i)
		}
	}
}

func TestMatcher(t *testing.T) {
	var m Matcher
	check := func(pattern, subject string, args ...interface{}) {
		checkmatch1(t, false, nil, pattern, subject, args...)
		checkmatch1(t, true, nil, pattern, subject, args...)
		checkmatch1(t, false, &m, pattern, subject, args...)
		checkmatch1(t, true, &m, pattern, subject, args...)
	}

	check(`^$`, "", "")
	check(`^abc$`, "abc", "abc")
	check(`^(X)*ab(c)$`, "abc", "abc", nil, "c")
	check(`^(X)*ab()c$`, "abc", "abc", nil, "")
	check(`^.*$`, "abc", "abc")
	check(`^.*$`, "a\000c", "a\000c")
	check(`^(.*)$`, "a\000c", "a\000c", "a\000c")
}

func TestNewMatcherJIT(t *testing.T) {
	re := MustCompileJIT(`\dG|internet|gprs|[Kk]b|[Mm]b|Gb|lte`, 0, STUDY_JIT_COMPILE)
	m := re.NewMatcherString(`4GKb`, 0)
	if !m.Matches {
		t.Error("The match should be matched")
	}
	m = re.NewMatcherString(`Some value`, 0)
	if m.Matches {
		t.Error("The match should not be matched")
	}
}

func TestCompileAndStudy(t *testing.T) {
	re, err := Compile(`(Bel[ao]rus)|(Бел[ао]рус)|(БЕЛ[АО]РУС)|Білорусь`, UTF8|CASELESS)
	if err != nil {
		t.Error("Compile error", err)
	}
	if len(re.extra) != 0 {
		t.Error("re.extra should be empty")
	}

	m := re.NewMatcherString("Беларусь: MTS", 0)
	if !m.Matches {
		t.Error("The match should be matched")
	}
	m = re.NewMatcherString("Other value", 0)
	if m.Matches {
		t.Error("The match should not be matched")
	}

	err = re.Study(0)
	if err != nil {
		t.Error("Study error", err)
	}
	if len(re.extra) == 0 {
		t.Error("re.extra should not be empty")
	}

	m = re.NewMatcherString("Беларусь: MTS", 0)
	if !m.Matches {
		t.Error("The match should be matched")
	}
	m = re.NewMatcherString("Other value", 0)
	if m.Matches {
		t.Error("The match should not be matched")
	}
}

func BenchmarkStudyAndExec(b *testing.B) {
	// Date check regexp
	re := MustCompile(`/^(?:(?:31(\/|-|\.)(?:0?[13578]|1[02]))\1|(?:(?:29|30)(\/|-|\.)(?:0?[1,3-9]|1[0-2])\2))(?:(?:1[6-9]|[2-9]\d)?\d{2})$|^(?:29(\/|-|\.)0?2\3(?:(?:(?:1[6-9]|[2-9]\d)?(?:0[48]|[2468][048]|[13579][26])|(?:(?:16|[2468][048]|[3579][26])00))))$|^(?:0?[1-9]|1\d|2[0-8])(\/|-|\.)(?:(?:0?[1-9])|(?:1[0-2]))\4(?:(?:1[6-9]|[2-9]\d)?\d{2})$/`, 0)
	err := re.Study(0)
	if err != nil {
		b.Error("Study error", err)
	}
	subj := []byte(`20-10-2023`)
	m := re.NewMatcher(subj, 0)
	for b.Loop() {
		m.MatchWFlags(subj, 0)
		if m == nil {
			b.Error("The match should be matched")
		}
	}
}

func BenchmarkExecJIT(b *testing.B) {
	// Date check regexp
	re := MustCompile(`/^(?:(?:31(\/|-|\.)(?:0?[13578]|1[02]))\1|(?:(?:29|30)(\/|-|\.)(?:0?[1,3-9]|1[0-2])\2))(?:(?:1[6-9]|[2-9]\d)?\d{2})$|^(?:29(\/|-|\.)0?2\3(?:(?:(?:1[6-9]|[2-9]\d)?(?:0[48]|[2468][048]|[13579][26])|(?:(?:16|[2468][048]|[3579][26])00))))$|^(?:0?[1-9]|1\d|2[0-8])(\/|-|\.)(?:(?:0?[1-9])|(?:1[0-2]))\4(?:(?:1[6-9]|[2-9]\d)?\d{2})$/`, 0)
	err := re.Study(STUDY_JIT_COMPILE)
	if err != nil {
		b.Error("Study error", err)
	}
	subj := []byte(`20-10-2023`)
	m := re.NewMatcher(subj, 0)
	for b.Loop() {
		m.MatchWFlags(subj, 0)
		if m == nil {
			b.Error("The match should be matched")
		}
	}
}

func BenchmarkExecWithoutStudy(b *testing.B) {
	// Date check regexp
	re := MustCompile(`/^(?:(?:31(\/|-|\.)(?:0?[13578]|1[02]))\1|(?:(?:29|30)(\/|-|\.)(?:0?[1,3-9]|1[0-2])\2))(?:(?:1[6-9]|[2-9]\d)?\d{2})$|^(?:29(\/|-|\.)0?2\3(?:(?:(?:1[6-9]|[2-9]\d)?(?:0[48]|[2468][048]|[13579][26])|(?:(?:16|[2468][048]|[3579][26])00))))$|^(?:0?[1-9]|1\d|2[0-8])(\/|-|\.)(?:(?:0?[1-9])|(?:1[0-2]))\4(?:(?:1[6-9]|[2-9]\d)?\d{2})$/`, 0)
	subj := []byte(`20-10-2023`)
	m := re.NewMatcher(subj, 0)
	for b.Loop() {
		m.MatchWFlags(subj, 0)
		if m == nil {
			b.Error("The match should be matched")
		}
	}
}

func TestPartial(t *testing.T) {
	re := MustCompile(`^abc`, 0)

	// Check we get a partial match when we should
	m := re.NewMatcherString("ab", PARTIAL_SOFT)
	if !m.Matches {
		t.Error("Failed to find any matches")
	} else if !m.Partial {
		t.Error("The match was not partial")
	}

	// Check we get an exact match when we should
	m = re.NewMatcherString("abc", PARTIAL_SOFT)
	if !m.Matches {
		t.Error("Failed to find any matches")
	} else if m.Partial {
		t.Error("Match was partial but should have been exact")
	}

	m = re.NewMatcher([]byte("ab"), PARTIAL_SOFT)
	if !m.Matches {
		t.Error("Failed to find any matches")
	} else if !m.Partial {
		t.Error("The match was not partial")
	}

	m = re.NewMatcher([]byte("abc"), PARTIAL_SOFT)
	if !m.Matches {
		t.Error("Failed to find any matches")
	} else if m.Partial {
		t.Error("The match was net partial")
	}
}

func TestCaseless(t *testing.T) {
	m := MustCompile("abc", CASELESS).NewMatcherString("Abc", 0)
	if !m.Matches {
		t.Error("CASELESS")
	}
	m = MustCompile("abc", 0).NewMatcherString("Abc", 0)
	if m.Matches {
		t.Error("!CASELESS")
	}
}

func TestNamed(t *testing.T) {
	m := MustCompile("(?<L>a)(?<M>X)*bc(?<DIGITS>\\d*)", 0).
		NewMatcherString("abc12", 0)

	if !m.Matches {
		t.Error("Matches")
	}

	if !m.NamedPresent("L") {
		t.Error("NamedPresent(\"L\")")
	}

	if m.NamedPresent("M") {
		t.Error("NamedPresent(\"M\")")
	}

	if !m.NamedPresent("DIGITS") {
		t.Error("NamedPresent(\"DIGITS\")")
	}

	group, err := m.NamedString("DIGITS")
	if err != nil || group != "12" {
		t.Error("NamedString(\"DIGITS\")")
	}
}

func TestNames(t *testing.T) {
	re := MustCompile("(?<name2>a)(b)(c)(?<n1>d)", 0)
	names := re.CaptureNames()
	if len(names) != 2 {
		t.Error("Names count", len(names))
	}
	if names[0].Name != "n1" {
		t.Error("Names name 1", names[0].Name)
	}
	if names[1].Name != "name2" {
		t.Error("Names name 2", names[1].Name)
	}
	if names[0].Index != 4 {
		t.Error("Names index 1", names[0].Index)
	}
	if names[1].Index != 1 {
		t.Error("Names index 2", names[1].Index)
	}

	// Have no named capturing
	re = MustCompile("(a)bc", 0)
	names = re.CaptureNames()
	if len(names) != 0 {
		t.Error("Names count", len(names))
	}
}

func TestFindIndex(t *testing.T) {
	re := MustCompile("bcd", 0)
	i := re.FindIndex([]byte("abcdef"), 0)
	if i[0] != 1 {
		t.Error("FindIndex start", i[0])
	}
	if i[1] != 4 {
		t.Error("FindIndex end", i[1])
	}
}

func TestExtract(t *testing.T) {
	re := MustCompile("b(c)(d)", 0)
	m := re.NewMatcher([]byte("abcdef"), 0)
	i := m.Extract()
	switch {
	case string(i[0]) != "abcdef":
		t.Error("Full line unavailable: ", i[0])
	case string(i[1]) != "c":
		t.Error("First match group no as expected: ", i[1])
	case string(i[2]) != "d":
		t.Error("Second match group no as expected: ", i[2])
	}
}

func TestExtractString(t *testing.T) {
	re := MustCompile("b(c)(d)", 0)
	m := re.NewMatcherString("abcdef", 0)
	i := m.ExtractString()
	switch {
	case i[0] != "abcdef":
		t.Error("Full line unavailable: ", i[0])
	case i[1] != "c":
		t.Error("First match group no as expected: ", i[1])
	case i[2] != "d":
		t.Error("Second match group no as expected: ", i[2])
	}
}

func TestReplaceAll(t *testing.T) {
	re := MustCompile("foo", 0)
	// Don't change at ends.
	result := re.ReplaceAll([]byte("I like foods."), []byte("car"), 0)
	if string(result) != "I like cards." {
		t.Error("ReplaceAll", result)
	}
	// Change at ends.
	result = re.ReplaceAll([]byte("food fight fools foo"), []byte("car"), 0)
	if string(result) != "card fight carls car" {
		t.Error("ReplaceAll2", result)
	}
}
