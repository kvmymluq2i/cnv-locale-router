// Package locale validates and case-normalizes BCP 47 language tags.
package locale

import "strings"

// Normalize reports whether tag is a well-formed BCP 47 language tag and, when
// it is, returns the tag with canonical casing: the language subtag is
// lower-cased, the script subtag is title-cased, the region is upper-cased, and
// every other subtag is lower-cased. Empty tags, tags with leading or trailing
// whitespace and tags violating the RFC 5646 grammar are rejected.
func Normalize(tag string) (string, bool) {
	if tag == "" || strings.TrimSpace(tag) != tag {
		return "", false
	}
	parts := strings.Split(tag, "-")
	for _, part := range parts {
		if part == "" {
			return "", false
		}
	}

	// Private-use tag standing on its own: x-private-subtag.
	if isX(parts[0]) {
		if len(parts) < 2 {
			return "", false
		}
		for _, part := range parts[1:] {
			if !isAlnum(part) || len(part) > 8 {
				return "", false
			}
		}
		return strings.ToLower(tag), true
	}

	out := make([]string, 0, len(parts))

	// language = 2*3ALPHA. Longer primary subtags and single letters are not
	// accepted; extlang is not implemented, so a 3-letter subtag in the region
	// position (e.g. "en-USA") is rejected rather than swallowed.
	primary := parts[0]
	if !isAlpha(primary) || len(primary) < 2 || len(primary) > 3 {
		return "", false
	}
	out = append(out, strings.ToLower(primary))
	i := 1

	// script = 4ALPHA
	if i < len(parts) && len(parts[i]) == 4 && isAlpha(parts[i]) {
		part := parts[i]
		out = append(out, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		i++
	}

	// region = 2ALPHA / 3DIGIT
	if i < len(parts) {
		if part := parts[i]; (len(part) == 2 && isAlpha(part)) || (len(part) == 3 && isDigits(part)) {
			out = append(out, strings.ToUpper(part))
			i++
		}
	}

	// variant = 5*8alphanum / (DIGIT 3alphanum)
	for i < len(parts) && isVariant(parts[i]) {
		out = append(out, strings.ToLower(parts[i]))
		i++
	}

	// *extension ["-" privateuse]
	seenSingletons := map[string]bool{}
	for i < len(parts) {
		part := parts[i]
		if len(part) != 1 || !isAlnum(part) {
			return "", false
		}
		if isX(part) {
			out = append(out, "x")
			if i == len(parts)-1 {
				return "", false
			}
			for _, sub := range parts[i+1:] {
				if !isAlnum(sub) || len(sub) > 8 {
					return "", false
				}
				out = append(out, strings.ToLower(sub))
			}
			break
		}

		singleton := strings.ToLower(part)
		if seenSingletons[singleton] {
			return "", false
		}
		seenSingletons[singleton] = true
		out = append(out, singleton)

		subtags := 0
		i++
		for i < len(parts) {
			sub := parts[i]
			if len(sub) == 1 && isAlnum(sub) {
				break
			}
			if !isAlnum(sub) || len(sub) < 2 || len(sub) > 8 {
				return "", false
			}
			out = append(out, strings.ToLower(sub))
			subtags++
			i++
		}
		if subtags == 0 {
			return "", false
		}
	}

	return strings.Join(out, "-"), true
}

func isVariant(part string) bool {
	if isAlnum(part) && len(part) >= 5 && len(part) <= 8 {
		return true
	}
	return len(part) == 4 && isDigit(part[0]) && isAlnum(part[1:])
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isAlphaByte(s[i]) {
			return false
		}
	}
	return s != ""
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return s != ""
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !isAlphaByte(b) && !isDigit(b) {
			return false
		}
	}
	return s != ""
}

func isAlphaByte(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isX(s string) bool { return s == "x" || s == "X" }
