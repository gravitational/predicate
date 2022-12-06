package predicate

import (
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
)

func TestMethods(t *testing.T) {
	t.Parallel()

	p, err := NewParser(Def{
		Functions: map[string]interface{}{
			"set":    newSet,
			"list":   newList,
			"append": customAppend,
		},
		Methods: map[string]interface{}{
			"add":      set.add,
			"append":   list.append,
			"contains": container.contains,
		},
		GetIdentifier: func(selector []string) (interface{}, error) {
			if len(selector) == 1 && selector[0] == "fruits" {
				return newSet("apples", "bananas"), nil
			}
			return nil, trace.BadParameter("unknown identifier %v", selector)
		},
	})
	require.NoError(t, err)

	for _, tc := range []struct {
		desc         string
		input        string
		expectError  bool
		expectOutput interface{}
	}{
		{
			desc:         "basic method call",
			input:        `set("a").add("b")`,
			expectOutput: newSet("a", "b"),
		},
		{
			desc:         "chained method calls",
			input:        `set("a").add("b").add("c")`,
			expectOutput: newSet("a", "b", "c"),
		},
		{
			desc:         "method call on identifier",
			input:        `fruits.add("cherries")`,
			expectOutput: newSet("apples", "bananas", "cherries"),
		},
		{
			desc:         "interface method on set",
			input:        `set("a", "b").contains("b")`,
			expectOutput: true,
		},
		{
			desc:         "interface method on list",
			input:        `list("a", "b").contains("b")`,
			expectOutput: true,
		},
		{
			desc:        "undefined method",
			input:       `set("a", "b").intersect(set("a"))`,
			expectError: true,
		},
		{
			desc:        "wrong receiver type",
			input:       `set("a", "b").append("c")`,
			expectError: true,
		},
		{
			desc:        "wrong argument type",
			input:       `set("a", "b").add(1)`,
			expectError: true,
		},
		{
			desc:        "too many arguments",
			input:       `set("a", "b").add("c", "d")`,
			expectError: true,
		},
		{
			desc:         "append as a method",
			input:        `list("a", "b").append("c")`,
			expectOutput: newList("a", "b", "c"),
		},
		{
			desc:         "append as a free function",
			input:        `append(list("a", "b"), "c")`,
			expectOutput: newList("a", "b", "c"),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			output, err := p.Parse(tc.input)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectOutput, output)
		})
	}
}

// set is an example set type used to demonstrate use of methods.
type set map[string]struct{}

func newSet(strings ...string) set {
	out := make(map[string]struct{}, len(strings))
	for _, s := range strings {
		out[s] = struct{}{}
	}
	return out
}

func (s set) contains(str string) bool {
	_, ok := s[str]
	return ok
}

func (s set) add(str string) set {
	newSet := set{}
	for k := range s {
		newSet[k] = struct{}{}
	}
	newSet[str] = struct{}{}
	return newSet
}

// list is an example list type used to demonstrate use of methods.
type list []string

func newList(strings ...string) list {
	return strings
}

func (l list) contains(str string) bool {
	for _, s := range l {
		if s == str {
			return true
		}
	}
	return false
}

func (l list) append(str string) list {
	return append(append([]string{}, l...), str)
}

// container is an example interface implemented by set and list to demonstrate
// use of interface methods.
type container interface {
	contains(str string) bool
}

func customAppend(l list, str string) list {
	return append(append([]string{}, l...), str)
}
