package provider

import "testing"

func TestPolicyAuthorize(t *testing.T) {
	p := Policy{Allowed: []Destination{{Host: "API.Example.com", Port: 443}}}
	if _, err := p.Authorize("api.example.com", 443); err != nil {
		t.Fatalf("authorized host rejected: %v", err)
	}
	if _, err := p.Authorize("api.example.com.", 443); err != nil {
		t.Fatalf("trailing-dot host rejected: %v", err)
	}
	if _, err := p.Authorize("api.example.com", 8443); err == nil {
		t.Fatal("unapproved port accepted")
	}
	if _, err := p.Authorize("evil.example.com", 443); err == nil {
		t.Fatal("unapproved host accepted")
	}
	if _, err := p.Authorize("api.example.com", 0); err == nil {
		t.Fatal("invalid port accepted")
	}
}

func TestPolicyCheckConfigured(t *testing.T) {
	if err := (Policy{}).CheckConfigured(); err == nil {
		t.Fatal("empty policy considered configured")
	}
	if err := (Policy{Allowed: []Destination{{Host: "api.example.com", Port: 443}}}).CheckConfigured(); err != nil {
		t.Fatalf("valid destination rejected: %v", err)
	}
	for _, bad := range []Destination{
		{Host: "127.0.0.1", Port: 443},
		{Host: "10.0.0.5", Port: 443},
		{Host: "::1", Port: 443},
		{Host: "169.254.169.254", Port: 80},
	} {
		if err := (Policy{Allowed: []Destination{bad}}).CheckConfigured(); err == nil {
			t.Fatalf("disallowed destination %+v accepted as configured", bad)
		}
	}
}
