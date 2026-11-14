package main

import (
	"strconv"
	"strings"
)

// compareVersions compares two module versions such as "v1.2.3" or the
// pseudo-version "v0.0.0-20260101000000-abcdef123456" by semver precedence,
// which orders pseudo-versions by their time. Build metadata such as
// "+incompatible" is ignored. ok is false if either is not a version.
func compareVersions(a, b string) (cmp int, ok bool) {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	if !okA || !okB {
		return 0, false
	}

	for i := range va.core {
		if va.core[i] != vb.core[i] {
			if va.core[i] < vb.core[i] {
				return -1, true
			}
			return 1, true
		}
	}

	// A version without a prerelease ranks above one with it.
	switch {
	case va.pre == "" && vb.pre == "":
		return 0, true
	case va.pre == "":
		return 1, true
	case vb.pre == "":
		return -1, true
	}
	return comparePrerelease(va.pre, vb.pre), true
}

type semver struct {
	core [3]uint64 // major, minor, patch
	pre  string
}

func parseVersion(v string) (semver, bool) {
	rest, ok := strings.CutPrefix(v, "v")
	if !ok {
		return semver{}, false
	}
	rest, _, _ = strings.Cut(rest, "+")
	rest, pre, hasPre := strings.Cut(rest, "-")
	if hasPre && pre == "" {
		return semver{}, false
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var out semver
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || len(p) > 1 && p[0] == '0' {
			return semver{}, false
		}
		out.core[i] = n
	}
	out.pre = pre
	return out, true
}

// comparePrerelease compares dot-separated identifiers: numbers numerically
// and below words, words in ASCII order, and a shorter list below a longer
// one it is a prefix of.
func comparePrerelease(a, b string) int {
	ia, ib := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(ia), len(ib)) {
		x, y := ia[i], ib[i]
		nx, errX := strconv.ParseUint(x, 10, 64)
		ny, errY := strconv.ParseUint(y, 10, 64)
		switch {
		case errX == nil && errY == nil:
			if nx != ny {
				if nx < ny {
					return -1
				}
				return 1
			}
		case errX == nil:
			return -1
		case errY == nil:
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(ia) < len(ib):
		return -1
	case len(ia) > len(ib):
		return 1
	}
	return 0
}
