package main

import "testing"

func TestParseTargets(t *testing.T) {
	targets, err := parseTargets(
		list{"open=http://gw/open/get", "keyed=https://gw/keyed/get?a=b"},
		list{"keyed=Authorization: lab-key", "keyed=X-Trace:on"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Name != "open" || targets[1].URL != "https://gw/keyed/get?a=b" {
		t.Fatalf("targets=%+v", targets)
	}
	if got := targets[1].Header.Get("Authorization"); got != "lab-key" {
		t.Errorf("Authorization=%q, want lab-key", got)
	}
	if got := targets[1].Header.Get("X-Trace"); got != "on" {
		t.Errorf("X-Trace=%q, want on", got)
	}
	if len(targets[0].Header) != 0 {
		t.Errorf("a header for one target must not reach another: %v", targets[0].Header)
	}
}

func TestParseTargetsRefuses(t *testing.T) {
	cases := map[string]struct{ urls, headers list }{
		"no url":                  {nil, nil},
		"url without a name":      {list{"http://gw/"}, nil},
		"url that is not http":    {list{"api=gw:8080"}, nil},
		"the same name twice":     {list{"api=http://a/", "api=http://b/"}, nil},
		"header for unknown name": {list{"api=http://a/"}, list{"other=K: v"}},
		"header without a colon":  {list{"api=http://a/"}, list{"api=Authorization"}},
		"header without a target": {list{"api=http://a/"}, list{"Authorization: v"}},
	}
	for name, c := range cases {
		if _, err := parseTargets(c.urls, c.headers); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
