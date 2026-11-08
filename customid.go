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

// add records id if it is a pattern. Patterns are tried in the order added.
func (r *customIDRouter) add(id string) {
	segments := strings.Split(id, ":")
	if !slices.ContainsFunc(segments, isCapture) {
		return
	}

	known := slices.ContainsFunc(*r, func(p customIDPattern) bool { return p.id == id })
	if !known {
		*r = append(*r, customIDPattern{id: id, segments: segments})
	}
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
