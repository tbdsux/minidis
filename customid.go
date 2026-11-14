package minidis

import (
	"slices"
	"strings"
)

// customIDRouter holds the handler IDs that contain captures. A pattern is
// split on ":", and a segment that is exactly "{name}" captures that segment of
// the custom ID, as in "vote:{poll}:{choice}". Any other segment is literal.
type customIDRouter []customIDPattern

type customIDPattern struct {
	id       string
	segments []string
}

// captureName returns the name of a "{name}" segment.
func captureName(segment string) (string, bool) {
	if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
		return segment[1 : len(segment)-1], true
	}
	return "", false
}

func isCapture(segment string) bool {
	_, ok := captureName(segment)
	return ok
}

// add records id if it is a pattern. Patterns with more literal segments are
// tried first, so "ticket:close:{id}" wins over "ticket:{action}:{id}"; among
// equals, the one added first.
func (r *customIDRouter) add(id string) {
	segments := strings.Split(id, ":")
	if !slices.ContainsFunc(segments, isCapture) {
		return
	}
	if slices.ContainsFunc(*r, func(p customIDPattern) bool { return p.id == id }) {
		return
	}

	p := customIDPattern{id: id, segments: segments}
	i := slices.IndexFunc(*r, func(q customIDPattern) bool { return q.literals() < p.literals() })
	if i < 0 {
		i = len(*r)
	}
	*r = slices.Insert(*r, i, p)
}

// literals counts the segments that are not captures.
func (p customIDPattern) literals() int {
	n := 0
	for _, s := range p.segments {
		if !isCapture(s) {
			n++
		}
	}
	return n
}

// match returns the ID of the first pattern that fits customID and its captures.
func (r customIDRouter) match(customID string) (string, map[string]string, bool) {
	segments := strings.Split(customID, ":")

	for _, p := range r {
		if params, ok := p.match(segments); ok {
			return p.id, params, true
		}
	}
	return "", nil, false
}

func (p customIDPattern) match(segments []string) (map[string]string, bool) {
	if len(segments) != len(p.segments) {
		return nil, false
	}

	params := map[string]string{}
	for i, want := range p.segments {
		name, capture := captureName(want)
		switch {
		case !capture && want != segments[i], capture && segments[i] == "":
			return nil, false
		case capture:
			params[name] = segments[i]
		}
	}
	return params, true
}

// idHandlers maps handler IDs, which may be patterns, to handlers.
type idHandlers[H any] struct {
	byID     map[string]H
	patterns customIDRouter
}

func (r *idHandlers[H]) add(id string, h H) {
	if r.byID == nil {
		r.byID = map[string]H{}
	}
	r.byID[id] = h
	r.patterns.add(id)
}

// find returns the handler for customID and the captures of its pattern.
func (r *idHandlers[H]) find(customID string) (H, map[string]string, bool) {
	if h, ok := r.byID[customID]; ok {
		return h, nil, true
	}
	if id, params, ok := r.patterns.match(customID); ok {
		return r.byID[id], params, true
	}

	var zero H
	return zero, nil, false
}
