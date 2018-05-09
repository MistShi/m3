// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package query

import (
	"fmt"
	re "regexp"

	"github.com/m3db/m3ninx/index"
	"github.com/m3db/m3ninx/search"
	"github.com/m3db/m3ninx/search/searcher"
)

// RegexpQuery finds documents which match the given regular expression.
type RegexpQuery struct {
	Field    []byte
	Regexp   []byte
	compiled *re.Regexp
}

// NewRegexpQuery constructs a new query for the given regular expression.
func NewRegexpQuery(field, regexp []byte) (search.Query, error) {
	compiled, err := re.Compile(string(regexp))
	if err != nil {
		return nil, err
	}

	return &RegexpQuery{
		Field:    field,
		Regexp:   regexp,
		compiled: compiled,
	}, nil
}

// Searcher returns a searcher over the provided readers.
func (q *RegexpQuery) Searcher(rs index.Readers) (search.Searcher, error) {
	return searcher.NewRegexpSearcher(rs, q.Field, q.Regexp, q.compiled), nil
}

func (q *RegexpQuery) String() string {
	return fmt.Sprintf("regexp(%s, %s)", q.Field, q.Regexp)
}