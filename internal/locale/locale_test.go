package locale

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"en", "en", true},
		{"EN", "en", true},
		{"en-us", "en-US", true},
		{"ZH-hans-cN", "zh-Hans-CN", true},
		{"zh-Hant-TW", "zh-Hant-TW", true},
		{"es-419", "es-419", true},
		{"pt-br", "pt-BR", true},
		{"sl-rozaj", "sl-rozaj", true},
		{"de-DE-1901", "de-DE-1901", true},
		{"en-u-ca-gregory", "en-u-ca-gregory", true},
		{"en-US-x-private", "en-US-x-private", true},
		{"x-whatever", "x-whatever", true},
		{"gan", "gan", true}, // 3-letter ISO 639-3 primary (preferred over zh-gan)
		{"ase", "ase", true}, // sign language as its own primary
		{"EN-GB-POSIX", "en-GB-posix", true},

		{"", "", false},
		{" ", "", false},
		{" en", "", false},
		{"en ", "", false},
		{"en_US", "", false},
		{"english", "", false},
		{"en-", "", false},
		{"-en", "", false},
		{"en--US", "", false},
		{"123", "", false},
		{"a", "", false},
		{"abcdefghi", "", false},
		{"en-USA", "", false},  // 3 letters in region position: not a 2-letter region
		{"zh-gan", "", false},  // extlang prefix rejected; use the primary "gan" instead
		{"sgn-ase", "", false}, // use the primary "ase" instead
		{"en-12", "", false},
		{"x", "", false},
		{"x-", "", false},
		{"en-u", "", false},
		{"en-u-a-u-b", "", false},
		{"en-abc-def-ghi-jkl", "", false},
		{"en US", "", false},
		{"en-us-", "", false},
	}

	for _, tc := range cases {
		got, ok := Normalize(tc.in)
		if ok != tc.ok {
			t.Errorf("Normalize(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
