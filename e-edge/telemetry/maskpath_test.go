package telemetry

import "testing"

func TestMaskPathHidesPersonalAndSecretParts(t *testing.T) {
	cases := map[string]string{
		"/users/an.nguyen@example.com/orders":              "/users/{email}/orders",
		"/orders/6f1c2a9e-3b4d-4e8a-9f10-2c7d5b1a8e44":     "/orders/{uuid}",
		"/customers/0912345678":                            "/customers/{number}",
		"/id/001203004567":                                 "/id/{number}",
		"/pay/4111 1111 1111 1111/status":                  "/pay/{number}/status",
		"/call/+84-912-345-678":                            "/call/{number}",
		"/auth/eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln":  "/auth/{token}",
		"/hooks/ghp_16C7e42F292c6912E7710c838347Ae178B4a":  "/hooks/{token}",
		"/d/9f86d081884c7d659a2feaa0c55ad015":              "/d/{token}",
		"/s/Zm9vYmFyMTIzNDU2Nzg5MGFiY2RlZg":                "/s/{token}",
		"/reset-password/k3j9x2":                           "/reset-password/{token}",
		"/account/Reset/abcdefghijklmnopqrst":              "/account/Reset/{token}",
		"/app;jsessionid=A1B2C3/home":                      "/app;jsessionid={value}/home",
		"/invite/9f86d081884c7d659a2feaa0c55ad015/accept/": "/invite/{token}/accept/",
	}
	for in, want := range cases {
		if got := MaskPath(in); got != want {
			t.Errorf("MaskPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// What an operator needs to read an event stays as it was, attack paths included.
func TestMaskPathKeepsOrdinaryPaths(t *testing.T) {
	for _, path := range []string{
		"", "/", "//double//slash/", "/api/v1/products/42", "/orders/1234567",
		"/blog/2026-11-28/release-notes", "/products/iphone-15-pro-max-256gb-black",
		"/password/reset/confirm", "/code/review", "/wp-admin/install.php",
		"/static/app.min.js", "/.env", "/cgi-bin/../../etc/passwd", "/search/' OR 1=1 --",
		"/InternationalizationSettings",
	} {
		if got := MaskPath(path); got != path {
			t.Errorf("MaskPath(%q) = %q, want it unchanged", path, got)
		}
	}
}

func TestRecordMasksThePathItQueues(t *testing.T) {
	telemetry := New(1)
	telemetry.Record(SecurityEvent{RequestPath: "/users/a@example.com"})
	if got := (<-telemetry.queue).RequestPath; got != "/users/{email}" {
		t.Fatalf("queued path = %q", got)
	}
}
