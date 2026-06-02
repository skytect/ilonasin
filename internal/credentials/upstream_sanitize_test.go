package credentials

import "testing"

func TestSanitizeOAuthDisplayAllowsEmailWithMultipleDots(t *testing.T) {
	email := "lynus@subs.pragnition.ai"
	if got := sanitizeOAuthDisplay(email, "access-token", "refresh-token"); got != email {
		t.Fatalf("sanitizeOAuthDisplay(%q) = %q, want original email", email, got)
	}
}

func TestLooksLikeJWTRejectsOnlyJWTShape(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"lynus@subs.pragnition.ai", false},
		{"example.com.with.dots", false},
		{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.signature", true},
		{"eyJ.invalid space.sig", false},
	}
	for _, tc := range cases {
		if got := looksLikeJWT(tc.value); got != tc.want {
			t.Fatalf("looksLikeJWT(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
