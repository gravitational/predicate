/*
Copyright 2014-2018 Vulcand Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

*/

/*
Package predicate used to create interpreted mini languages with Go syntax - mostly to define
various predicates for configuration, e.g. Latency() > 40 || ErrorRate() > 0.5.

Here's an example of fully functional predicate language to deal with division remainders:

	    // takes number and returns true or false
	    type numberPredicate func(v int) bool

	    // Converts one number to another
	    type numberMapper func(v int) int

	    // Function that creates predicate to test if the remainder is 0
	    func divisibleBy(divisor int) numberPredicate {
		    return func(v int) bool {
			    return v%divisor == 0
	        }
	    }

	    // Function - logical operator AND that combines predicates
	    func numberAND(a, b numberPredicate) numberPredicate {
	        return func(v int) bool {
	            return a(v) && b(v)
	        }
	    }

	    p, err := NewParser(Def{
			Operators: Operators{
				AND: numberAND,
			},
			Functions: map[string]interface{}{
				"DivisibleBy": divisibleBy,
			},
		})

		pr, err := p.Parse("DivisibleBy(2) && DivisibleBy(3)")
	    if err == nil {
	        fmt.Fatalf("Error: %v", err)
	    }
	    pr.(numberPredicate)(2) // false
	    pr.(numberPredicate)(3) // false
	    pr.(numberPredicate)(6) // true
*/
package predicate

// Def defines parser context including supported operators, functions, methods,
// identifiers, and property accessors.
type Def struct {
	Operators Operators
	// Function matching is case sensitive, e.g. Len is different from len
	Functions map[string]any
	// Methods is a map of method names to their implementation.
	Methods map[string]any
	// GetIdentifier returns value of any identifier passed in the form
	// []string{"id", "field", "subfield"}
	GetIdentifier GetIdentifierFn
	// GetProperty returns property from a map
	GetProperty GetPropertyFn
}

// GetIdentifierFn function returns identifier based on selector
// e.g. id.field.subfield will be passed as.
// GetIdentifierFn([]string{"id", "field", "subfield"}).
type GetIdentifierFn func(selector []string) (any, error)

// GetPropertyFn returns property from a mapVal by key keyVal.
type GetPropertyFn func(mapVal, keyVal any) (any, error)

// Operators contain functions for equality and logical comparison.
type Operators struct {
	EQ  any
	NEQ any

	LT any
	GT any

	LE any
	GE any

	OR  any
	AND any
	NOT any
}

// Parser takes the string with expression and calls the operators and functions.
type Parser interface {
	Parse(string) (any, error)
}
