package state

import "testing"

func TestLocationRules(t *testing.T) {
	P := func(s State) Member { return Member{Primary: true, State: s} }
	S := func(s State) Member { return Member{State: s} }
	cases := []struct {
		name       string
		m          []Member
		want       State
		incomplete bool
	}{
		{"all up", []Member{P(Up), S(Up)}, Up, false},
		{"secondary down", []Member{P(Up), S(Down)}, Degraded, false},
		{"primary down", []Member{P(Down), S(Up)}, Down, false},
		{"primary down other unknown", []Member{P(Down), P(Unknown)}, Down, true},
		{"primary unknown", []Member{P(Unknown), S(Up)}, Unknown, true},
		{"secondary unknown", []Member{P(Up), S(Unknown)}, Unknown, true},
		{"degraded incomplete", []Member{P(Up), S(Down), S(Unknown)}, Degraded, true},
		{"primary error", []Member{P(Error), S(Up)}, Unknown, true},
		{"empty", nil, Unknown, true},
		{"no primary", []Member{S(Up)}, Unknown, true},
	}
	for _, c := range cases {
		got := Location(c.m)
		if got.State != c.want {
			t.Errorf("%s: state %s want %s", c.name, got.State, c.want)
		}
		if c.name == "degraded incomplete" && !got.Incomplete {
			t.Errorf("%s: want incomplete", c.name)
		}
		if c.name == "all up" && got.Incomplete {
			t.Errorf("%s: unexpected incomplete", c.name)
		}
	}
}
