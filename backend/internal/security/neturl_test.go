package security

import "testing"

func TestValidatePublicHTTPURL(t *testing.T) {
	t.Setenv("BASE_WEBHOOK_ALLOW_PRIVATE", "")
	cases := []struct {
		url  string
		want bool
	}{
		{"https://8.8.8.8/hook", true},
		{"http://1.1.1.1/hook", true},
		{"http://127.0.0.1/hook", false},
		{"http://localhost/hook", false},
		{"http://10.0.0.1/hook", false},
		{"http://169.254.169.254/latest", false},
		{"ftp://8.8.8.8/x", false},
		{"https://user:pass@8.8.8.8/", false},
	}
	for _, tc := range cases {
		err := ValidatePublicHTTPURL(tc.url)
		ok := err == nil
		if ok != tc.want {
			t.Fatalf("%s: got ok=%v err=%v want ok=%v", tc.url, ok, err, tc.want)
		}
	}
}

func TestValidatePublicHTTPURLAllowPrivate(t *testing.T) {
	t.Setenv("BASE_WEBHOOK_ALLOW_PRIVATE", "1")
	if err := ValidatePublicHTTPURL("http://127.0.0.1/hook"); err != nil {
		t.Fatal(err)
	}
}
